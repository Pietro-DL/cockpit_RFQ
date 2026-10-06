// L1 — gli ancoraggi dei file e la pre-associazione (piano A, 6.4.5, regole 3-8; par.3.3.7; contratto §1.5, §2.2,
// §7 famiglia B4; commit P6b, fase 2): dai fatti del worker (STEP e PDF) all'adattatore, a Interpreta, a StrutturaDa e
// a ProponiAncoraggi, che chiama ProponiStrutture. I candidati a livello prodotto e componente con le radici
// raggiungibili, i padri, i percorsi e gli archi (FIGLIO-CONDIVISO), l'associazione e la collocazione con i motivi, la
// pre-associazione sulla BOM di lavoro (R85), l'incertezza dell'identità accanto (T-E1-03, PO-20), la compatibilità
// con le decisioni che si ricalcola (T-E1-05, PO-21), SenzaFile ricalcolato, HASH-CONFLITTO, le impronte (A-C11), il
// determinismo; A1c-L1-07…13 e -15 nelle parti dei file; l'abbinamento per base dei nodi (emendamento E1 §4.2).
package ancoraggio_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

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
// un cliente cambia convenzione. Il cliente è ACME (acme.example), i codici sono di fantasia (712xxxx, con il marcatore
// A e la revisione di una cifra nella famiglia acme-catena, e la base a punti 9.xxx.xxxx.x), i nomi dei file hanno il
// prefisso di progetto inventato «ACME-030P» e lo stato PDM generico «IN_WORK», gli UUID sono
// 00000000-0000-4000-8000-0000000000nn. La scena è quella delle strutture (struttura_test.go), derivata dallo scenario
// sintetico delle prove del giro 4, con i PDF dei pezzi. Le prove citano i requisiti (A1c-L1-07, R85, T-E1-03,
// PO-20, PO-21), mai i casi degli attesi.

// ---- la grammatica ACME degli ancoraggi ----

// famAncoraggi: la famiglia delle strutture (base 712 più quattro cifre), che legge anche il testo libero dei PDF (per
// avere le menzioni: A1c-L1-10), più una forma del cartiglio con la base ripetuta, «7120110_R7120110», per una base
// ripetuta che non concorda (D2: A1c-L1-13). La forma del codice ha il confine di parola dopo la base, così non legge
// la stessa occorrenza della forma ripetuta (R25 f).
func famAncoraggi() grammatica.FamigliaCodice {
	f := famStrutture()
	parola := grammatica.ConfineParolaASCII
	f.Forme[1].Selettori = append(append([]string(nil), selStrutture...), "testo_pdf")
	f.Forme[1].ConfineDopo = &parola
	f.Forme = append(f.Forme, grammatica.FormaCodice{ID: "ripetuta", Selettori: []string{"cartiglio.codice"}, Stato: grammatica.StatoAttiva, Completa: true,
		Parti: []grammatica.Parte{{Tipo: grammatica.TipoParteBase, Min: 1, Max: 1}, {Tipo: grammatica.TipoParteSeparatore, Letterali: []string{"_R"}, Min: 1, Max: 1},
			{Tipo: grammatica.TipoParteRipetizioneBase, Min: 1, Max: 1}}})
	f.Esempi = append(append([]grammatica.EsempioCodice(nil), f.Esempi...),
		esempio("e-ripetuta", "cartiglio.codice", "7120110_R7120110", true, grammatica.LetturaAttesa{Forma: "ripetuta", Base: "7120110"}))
	return f
}

func motoreAncoraggi(t *testing.T) *motorea.Motore {
	t.Helper()
	return motore(t, famAncoraggi())
}

// famPuntiAncoraggi: la base a punti ACME (A-C07) con la forma completa e la parziale anche sul cartiglio.
func famPuntiAncoraggi() grammatica.FamigliaCodice {
	f := famPuntiStrutture()
	for i := range f.Forme {
		f.Forme[i].Selettori = append(append([]string(nil), selPuntiStrutture...), "cartiglio.codice")
	}
	return f
}

// ---- i file ----

// conDisponibilita: il file con la disponibilità che darà valutazione.
func conDisponibilita(f ancoraggio.FileInterpretato, d ancoraggio.Disponibilita) ancoraggio.FileInterpretato {
	f.Disponibilita = d
	return f
}

// stepF: uno STEP d'assieme o di un pezzo, come stepS, con il file e la sua struttura.
func stepF(t *testing.T, m *motorea.Motore, id uuid.UUID, sha string, nodi []nodoS, archi []arcoS, motivo string) (ancoraggio.FileInterpretato, ancoraggio.StrutturaFile) {
	t.Helper()
	f := fileS(t, m, id, "ACME-030P"+nodi[0].id+" 00 IN_WORK.stp", "stp", sha, fattiS(t, sha, []string{nodi[0].chiave}, nodi, archi, &motivo))
	return conDisponibilita(f, ancoraggio.DisponibilitaDisponibile), strutturaS(t, f)
}

// campoP: un campo del cartiglio di un PDF, con l'etichetta del worker.
type campoP struct{ etichetta, valore string }

// pdfF: un PDF con il testo v2 del worker: i campi del cartiglio e i frammenti di testo libero dati. Senza campi né
// frammenti è una scansione senza testo nativo e senza OCR.
func pdfF(t *testing.T, m *motorea.Motore, id uuid.UUID, nome, sha string, campi []campoP, frammenti ...string) ancoraggio.FileInterpretato {
	t.Helper()
	var cs, fs []map[string]any
	for _, c := range campi {
		cs = append(cs, map[string]any{"etichetta": c.etichetta, "letta": strings.ToUpper(c.etichetta), "valore": c.valore, "pagina": 1, "zona": "basso_destra",
			"fonte": "nativo", "riquadro": []float64{800, 700, 900, 711}, "confidenza": nil})
	}
	for _, x := range frammenti {
		fs = append(fs, map[string]any{"pagina": 1, "zona": "basso_destra", "fonte": "nativo", "testo": x, "riquadro": []float64{800, 690, 900, 720}, "confidenza": nil})
	}
	estraibile, ocr := true, "non_necessario"
	if len(cs) == 0 && len(fs) == 0 {
		estraibile, ocr = false, "non_disponibile"
	}
	if cs == nil {
		cs = []map[string]any{}
	}
	if fs == nil {
		fs = []map[string]any{}
	}
	raw, err := json.Marshal(map[string]any{"esito": map[string]any{"tipo_proposto": "disegno_2d"}, "testo_pdf": map[string]any{
		"versione": 2, "estraibile": estraibile, "pagine": 1, "pagine_lette": 1, "caratteri": 10, "troncato": false, "formato_pagina1": []float64{1191, 842},
		"frammenti": fs, "cartiglio": cs, "metadati": map[string]any{"titolo": "", "soggetto": "", "parole_chiave": "", "creatore": "", "produttore": ""},
		"ocr": map[string]any{"stato": ocr, "motivo": "", "motore": "", "tentativi": []string{}},
		"limiti": map[string]any{"pagine_max": 11, "frammenti_max": 200, "frammento_max": 512, "caratteri_max": 16384, "campi_max": 24,
			"ocr_soglia_pagina": 20, "ocr_soglia_cartiglio": 8, "ocr_pagine_max": 2, "ocr_tempo_max_s": 60, "ocr_dpi": 300},
	}})
	if err != nil {
		t.Fatal(err)
	}
	f := fileS(t, m, id, nome, "pdf", sha, fattiGrezzi(t, sha, raw, nil))
	d := ancoraggio.DisponibilitaDisponibile
	if !estraibile {
		d = ancoraggio.DisponibilitaSenzaTesto
	}
	return conDisponibilita(f, d)
}

// cartiglio: un PDF con il codice del cartiglio dato e un nome che nessuna famiglia legge.
func cartiglio(t *testing.T, m *motorea.Motore, n int, codice string, frammenti ...string) ancoraggio.FileInterpretato {
	t.Helper()
	return pdfF(t, m, uidS(n), "disegno-"+uidS(n).String()[33:]+".pdf", shaS(string("0123456789abcdef"[n%16]))[:63]+string("0123456789abcdef"[(n/16)%16]),
		[]campoP{{"codice", codice}}, frammenti...)
}

// ---- le chiamate, con gli invarianti di ogni esito ----

// ancoraA chiama ProponiAncoraggi e controlla gli invarianti di ogni esito:
//   - gli ingressi di chi chiama non cambiano;
//   - un ancoraggio per file, in ordine di allegato, con la disponibilità del file;
//   - l'associazione dice i candidati (nessuno, uno, più), salvo discordante; mai non_valutata; la collocazione radice
//     solo con candidati prodotto, figlio solo con candidati componente, fuori_richiesta senza candidati e senza
//     motivi, non_determinabile sempre con un motivo;
//   - ogni candidato è proposto (mai una conferma: R29 f, T-B0-09), con l'autorità di un target, le posizioni su nodi
//     delle strutture e le radici raggiungibili che sono i target delle sue posizioni;
//   - le strutture sono quelle di ProponiStrutture, salvo SenzaFile, che è falso solo per un nodo con un file candidato
//     o per la radice del suo STEP (3.3.7, P-20), e salvo la riconciliazione (fase 3: il codice documentale, i
//     candidati del cartiglio, lo stato della revisione), con i suoi invarianti (controllaRiconciliazione);
//   - le diagnostiche sono quelle di ProponiStrutture, più quelle della riconciliazione;
//   - le impronte ci sono.
func ancoraA(t *testing.T, file []ancoraggio.FileInterpretato, target []ancoraggio.ProdottoRichiesto, ctx ancoraggio.ContestoStrutturale) ancoraggio.EsitoAncoraggi {
	t.Helper()
	primaF, primaT, primaC := canonico(t, file), canonico(t, target), canonico(t, ctx)
	e, err := ancoraggio.ProponiAncoraggi(file, target, ctx)
	if err != nil {
		t.Fatalf("ProponiAncoraggi: %v", err)
	}
	if canonico(t, file) != primaF || canonico(t, target) != primaT || canonico(t, ctx) != primaC {
		t.Error("ProponiAncoraggi ha cambiato gli ingressi di chi chiama")
	}
	if e.VersioneServizio != ancoraggio.VersioneServizio || len(e.HashIngresso) != 64 || len(e.HashTarget) != 64 || len(e.Impronta) != 64 {
		t.Errorf("versione e impronte: %q %q %q %q", e.VersioneServizio, e.HashIngresso, e.HashTarget, e.Impronta)
	}
	if len(e.File) != len(file) {
		t.Fatalf("%d ancoraggi per %d file", len(e.File), len(file))
	}
	autorita := map[string]ancoraggio.Autorita{}
	for _, x := range target {
		autorita[x.Rif] = x.Autorita
	}
	perAllegato := map[uuid.UUID]ancoraggio.FileInterpretato{}
	for _, f := range file {
		perAllegato[f.AllegatoID] = f
	}
	nodi := map[string]bool{}
	for _, s := range e.Strutture {
		for _, n := range s.Nodi {
			nodi[s.Target+"|"+s.AllegatoID.String()+"|"+s.Radice+"|"+n.Rif] = true
		}
	}
	ancorati := map[string]bool{}
	for i, a := range e.File {
		if i > 0 && e.File[i-1].AllegatoID.String() >= a.AllegatoID.String() {
			t.Errorf("ancoraggi fuori ordine: %s dopo %s", a.AllegatoID, e.File[i-1].AllegatoID)
		}
		if a.Disponibilita != perAllegato[a.AllegatoID].Disponibilita {
			t.Errorf("%s: disponibilità %q, il file dice %q", a.AllegatoID, a.Disponibilita, perAllegato[a.AllegatoID].Disponibilita)
		}
		switch a.Associazione {
		case ancoraggio.AssociazioneDiscordante:
		case ancoraggio.AssociazioneNessunCandidato:
			if len(a.Candidati) != 0 {
				t.Errorf("%s: nessun candidato con %d candidati", a.AllegatoID, len(a.Candidati))
			}
		case ancoraggio.AssociazioneCandidatoUnico:
			if len(a.Candidati) != 1 {
				t.Errorf("%s: candidato unico con %d candidati", a.AllegatoID, len(a.Candidati))
			}
		case ancoraggio.AssociazioneAmbiguo:
			if len(a.Candidati) < 2 {
				t.Errorf("%s: ambiguo con %d candidati", a.AllegatoID, len(a.Candidati))
			}
		default:
			t.Errorf("%s: associazione %q", a.AllegatoID, a.Associazione)
		}
		livelli := map[string]bool{}
		for _, c := range a.Candidati {
			livelli[c.Livello] = true
		}
		switch a.Collocazione {
		case ancoraggio.CollocazioneRadice:
			if len(livelli) != 1 || !livelli[ancoraggio.LivelloProdotto] {
				t.Errorf("%s: radice con i livelli %v", a.AllegatoID, livelli)
			}
		case ancoraggio.CollocazioneFiglio:
			if len(livelli) != 1 || !livelli[ancoraggio.LivelloComponente] {
				t.Errorf("%s: figlio con i livelli %v", a.AllegatoID, livelli)
			}
		case ancoraggio.CollocazioneFuoriRichiesta:
			if len(a.Candidati) != 0 || len(target) == 0 {
				t.Errorf("%s: fuori richiesta con candidati o senza target: %+v", a.AllegatoID, a)
			}
		case ancoraggio.CollocazioneNonDeterminabile:
			if len(a.Motivi) == 0 {
				t.Errorf("%s: non determinabile senza motivi", a.AllegatoID)
			}
		default:
			t.Errorf("%s: collocazione %q", a.AllegatoID, a.Collocazione)
		}
		for _, c := range a.Candidati {
			if c.Origine != ancoraggio.OrigineProposto || c.Target == "" || len(c.Letture) == 0 || len(c.Dimensioni) == 0 {
				t.Errorf("%s: candidato %+v", a.AllegatoID, c)
			}
			if c.Autorita != ancoraggio.AutoritaConfermata && c.Autorita != ancoraggio.AutoritaScenario {
				t.Errorf("%s: candidato con l'autorità %q", a.AllegatoID, c.Autorita)
			}
			var raggiungibili []string
			for _, p := range c.Posizioni {
				k := p.Target + "|" + p.AllegatoID.String() + "|" + p.Radice + "|" + p.Nodo
				if !nodi[k] {
					t.Errorf("%s: posizione fuori dalle strutture %+v", a.AllegatoID, p)
				}
				ancorati[k] = true
				if !contiene(raggiungibili, p.Target) {
					raggiungibili = append(raggiungibili, p.Target)
				}
			}
			if c.Livello == ancoraggio.LivelloComponente {
				if len(c.Posizioni) == 0 || strings.Join(ordinateS(raggiungibili), ",") != strings.Join(c.RadiciRaggiungibili, ",") {
					t.Errorf("%s: radici raggiungibili %v, le posizioni dicono %v", a.AllegatoID, c.RadiciRaggiungibili, raggiungibili)
				}
			} else if _, ok := autorita[c.Target]; !ok || c.Autorita != autorita[c.Target] || !reflect.DeepEqual(c.RadiciRaggiungibili, []string{c.Target}) {
				t.Errorf("%s: candidato prodotto %+v", a.AllegatoID, c)
			}
			for _, d := range c.Dimensioni {
				if d.Esito == motorea.CompatibilitaDiscordante && !contiene(c.Conflitti, d.Dimensione) {
					t.Errorf("%s: dimensione discordante %s senza il conflitto", a.AllegatoID, d.Dimensione)
				}
			}
		}
	}
	strutture, diag := proponiS(t, target, ctx)
	attese := canonico(t, strutture)
	senza := append([]ancoraggio.StrutturaProdotto(nil), e.Strutture...)
	for i := range senza {
		senza[i].Nodi = append([]ancoraggio.NodoProposto(nil), e.Strutture[i].Nodi...)
		for j := range senza[i].Nodi {
			n := senza[i].Nodi[j]
			k := senza[i].Target + "|" + senza[i].AllegatoID.String() + "|" + senza[i].Radice + "|" + n.Rif
			if n.SenzaFile == (ancorati[k] || (n.Rif == senza[i].Radice && senza[i].RadiceDelFile)) {
				t.Errorf("nodo %s della struttura di %s: SenzaFile %v", n.Rif, senza[i].Target, n.SenzaFile)
			}
			senza[i].Nodi[j].SenzaFile = true
			senza[i].Nodi[j].Codice = senzaRiconciliazione(n.Codice)
		}
	}
	if len(senza) == 0 {
		senza = nil
	}
	// Riscritta per la fase 3: la riconciliazione aggiunge ai nodi solo il codice documentale, i candidati del
	// cartiglio (con lo stato della revisione che ne segue) e le sue diagnostiche; tutto il resto è di ProponiStrutture,
	// quindi niente si applica (R64 A, R87).
	var diagStrutture []evidenze.Diagnostica
	for _, d := range e.Diagnostiche {
		if d.Codice != ancoraggio.CodiceCompletamentoDocumentale && d.Codice != ancoraggio.CodiceCorrezioneDocumentale {
			diagStrutture = append(diagStrutture, d)
		}
	}
	if canonico(t, senza) != attese || canonico(t, diagStrutture) != canonico(t, diag) {
		t.Errorf("le strutture o le diagnostiche dell'esito non sono quelle di ProponiStrutture:\n%s\n%s", canonico(t, senza), attese)
	}
	controllaRiconciliazione(t, e, file, ctx)
	return e
}

func ordinateS(s []string) []string {
	out := append([]string(nil), s...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// ancoraggioDi: l'ancoraggio di un allegato.
func ancoraggioDi(t *testing.T, e ancoraggio.EsitoAncoraggi, id uuid.UUID) ancoraggio.AncoraggioFile {
	t.Helper()
	for _, a := range e.File {
		if a.AllegatoID == id {
			return a
		}
	}
	t.Fatalf("nessun ancoraggio per %s", id)
	return ancoraggio.AncoraggioFile{}
}

// targetDei: i Target dei candidati, nell'ordine dell'ancoraggio.
func targetDei(a ancoraggio.AncoraggioFile) []string {
	var out []string
	for _, c := range a.Candidati {
		out = append(out, c.Target)
	}
	return out
}

// dimensione: l'esito di una dimensione di un candidato verso un riferimento; "" se non c'è.
func dimensione(c ancoraggio.CandidatoAncoraggio, nome, rif string) motorea.Compatibilita {
	for _, d := range c.Dimensioni {
		if d.Dimensione == nome && d.Rif == rif {
			return d.Esito
		}
	}
	return ""
}

// strutturaDiE: la struttura di un target in un allegato.
func strutturaDiE(t *testing.T, e ancoraggio.EsitoAncoraggi, target string, allegato uuid.UUID) ancoraggio.StrutturaProdotto {
	t.Helper()
	for _, s := range e.Strutture {
		if s.Target == target && s.AllegatoID == allegato {
			return s
		}
	}
	t.Fatalf("nessuna struttura di %s in %s", target, allegato)
	return ancoraggio.StrutturaProdotto{}
}

// ---- la scena ACME con i file (A1c-L1-07…09) ----

// Gli allegati della scena: i cinque STEP della scena delle strutture (tre assiemi, due chiesti; due pezzi «solo
// parti») e i PDF dei pezzi e del prodotto.
var (
	idPDF112 = uidS(0x711) // 7120112, il pezzo in comune fra i due assiemi
	idPDF122 = uidS(0x712) // 7120122, un figlio del solo 7120103
	idPDF120 = uidS(0x713) // 7120120, un figlio dell'assieme non chiesto
	idPDF100 = uidS(0x714) // 7120100, il prodotto
)

// scenaFileACME: i file della scena e le loro strutture, con il motivo di completezza dell'assieme 7120100.
func scenaFileACME(t *testing.T, m *motorea.Motore, motivo100 string) ([]ancoraggio.FileInterpretato, []ancoraggio.StrutturaFile) {
	t.Helper()
	var file []ancoraggio.FileInterpretato
	var strutture []ancoraggio.StrutturaFile
	aggiungi := func(f ancoraggio.FileInterpretato, s ancoraggio.StrutturaFile) {
		file = append(file, f)
		strutture = append(strutture, s)
	}
	aggiungi(stepF(t, m, idAssieme100, shaAssieme100,
		[]nodoS{{"#1", "7120100", "TELAIO ACME"}, {"#2", "7120110", "PIASTRA"}, {"#3", "7120111", "LAMIERA"}, {"#4", "7120112", "TONDO"}, {"#5", "SALDATURA_1", ""}},
		[]arcoS{{"#1", "#2", 1, []string{"#21"}}, {"#1", "#3", 1, []string{"#22"}}, {"#1", "#4", 2, []string{"#23", "#24"}}, {"#1", "#5", 1, nil}}, motivo100))
	aggiungi(stepF(t, m, idAssieme103, shaAssieme103,
		[]nodoS{{"#1", "7120103", "STAFFA ACME"}, {"#2", "7120122", "SQUADRA"}, {"#3", "7120133", "VITE"}, {"#4", "7120112", "TONDO"}},
		[]arcoS{{"#1", "#2", 1, nil}, {"#1", "#3", 2, []string{"#31", "#32"}}, {"#1", "#4", 1, nil}}, ""))
	aggiungi(stepF(t, m, idAssieme102, shaAssieme102,
		[]nodoS{{"#1", "7120102", "SUPPORTO ACME"}, {"#2", "7120120", "PIASTRA"}, {"#3", "7120121", "BOCCOLA"}},
		[]arcoS{{"#1", "#2", 1, nil}, {"#1", "#3", 1, nil}}, ""))
	aggiungi(stepF(t, m, idPezzo110, shaPezzo110, []nodoS{{"#1", "7120110", "PIASTRA"}}, nil, motivoSoloParti))
	aggiungi(stepF(t, m, idPezzo112, shaPezzo112, []nodoS{{"#1", "7120112", "TONDO"}}, nil, motivoSoloParti))
	file = append(file,
		pdfF(t, m, idPDF112, "7120112.pdf", shaS("b"), []campoP{{"codice", "7120112"}}),
		pdfF(t, m, idPDF122, "7120122.pdf", shaS("c"), []campoP{{"codice", "7120122"}}),
		pdfF(t, m, idPDF120, "7120120.pdf", shaS("d"), []campoP{{"codice", "7120120"}}),
		pdfF(t, m, idPDF100, "7120100.pdf", shaS("e"), []campoP{{"codice", "7120100"}}))
	return file, strutture
}

// ---- A1c-L1-07: la scena ----

// TestL107AncoraggiDellaScena (A1c-L1-07; 6.4.5 regola 3; v3 §2 r.160-166): due assiemi chiesti con l'autorità dello
// scenario e uno non chiesto.
//   - Gli STEP dei due assiemi chiesti e il PDF del prodotto sono radice, ognuno sul suo target, con le radici delle
//     strutture fra le posizioni.
//   - Il pezzo di un solo assieme (lo STEP «solo parti» del pezzo) è figlio, sul nodo: radice raggiungibile, padre,
//     percorso, arco con la quantità e l'occorrenza.
//   - Il pezzo in comune (lo STEP e il PDF) è UN candidato con due radici raggiungibili, due padri, due percorsi, due
//     archi (FIGLIO-CONDIVISO): sta su nodi di due STEP diversi, quindi il Target è l'identità (T-E1-04).
//   - L'assieme non chiesto e il PDF del suo figlio sono fuori richiesta: il grafo dei target è completo, e i file «solo
//     parti» dei figli non contano.
//   - Autorità scenario su tutti; SenzaFile solo sui nodi senza file.
func TestL107AncoraggiDellaScena(t *testing.T) {
	m := motoreAncoraggi(t)
	file, strutture := scenaFileACME(t, m, "")
	target := targetScenaACME(t, m)
	e := ancoraA(t, file, target, ancoraggio.ContestoStrutturale{Strutture: strutture})
	t1, t2 := "scenario:caso-acme:1", "scenario:caso-acme:2"
	r100 := func(k string) string { return ancoraggio.RifNodo(shaAssieme100, k) }
	r103 := func(k string) string { return ancoraggio.RifNodo(shaAssieme103, k) }

	conteggi := map[ancoraggio.Collocazione]int{}
	for _, a := range e.File {
		conteggi[a.Collocazione]++
		for _, c := range a.Candidati {
			if c.Autorita != ancoraggio.AutoritaScenario {
				t.Errorf("%s: autorità %q, lo scenario dà scenario a tutti (R75 A)", a.AllegatoID, c.Autorita)
			}
		}
	}
	if conteggi[ancoraggio.CollocazioneRadice] != 3 || conteggi[ancoraggio.CollocazioneFiglio] != 4 || conteggi[ancoraggio.CollocazioneFuoriRichiesta] != 2 {
		t.Errorf("conteggi della scena: %v, attesi 3 radice, 4 figlio, 2 fuori richiesta", conteggi)
	}

	for _, c := range []struct {
		id     uuid.UUID
		target string
		radice string
	}{{idAssieme100, t1, r100("#1")}, {idAssieme103, t2, r103("#1")}, {idPDF100, t1, r100("#1")}} {
		a := ancoraggioDi(t, e, c.id)
		if a.Associazione != ancoraggio.AssociazioneCandidatoUnico || a.Collocazione != ancoraggio.CollocazioneRadice || len(a.Motivi) != 0 ||
			a.Candidati[0].Target != c.target || a.Candidati[0].Livello != ancoraggio.LivelloProdotto || len(a.Candidati[0].Posizioni) != 1 ||
			a.Candidati[0].Posizioni[0].Nodo != c.radice || a.Candidati[0].Posizioni[0].Stato != ancoraggio.StatoStrutturaCandidata {
			t.Errorf("%s: radice di %s attesa: %+v", c.id, c.target, a)
		}
		if dimensione(a.Candidati[0], ancoraggio.DimensioneBase, c.target) != motorea.CompatibilitaUguale {
			t.Errorf("%s: la dimensione della base: %+v", c.id, a.Candidati[0].Dimensioni)
		}
	}
	if a := ancoraggioDi(t, e, idAssieme100); !reflect.DeepEqual(a.Candidati[0].RadiciDelFile, []string{r100("#1")}) {
		t.Errorf("le radici del file dello STEP: %v", a.Candidati[0].RadiciDelFile)
	}
	if a := ancoraggioDi(t, e, idPDF100); a.Candidati[0].RadiciDelFile != nil {
		t.Errorf("un PDF non ha radici del file: %v", a.Candidati[0].RadiciDelFile)
	}

	uno, due := 1, 2
	p110 := ancoraggioDi(t, e, idPezzo110)
	if p110.Associazione != ancoraggio.AssociazioneCandidatoUnico || p110.Collocazione != ancoraggio.CollocazioneFiglio {
		t.Fatalf("il pezzo di un solo assieme: %+v", p110)
	}
	c110 := p110.Candidati[0]
	if c110.Target != r100("#2") || c110.Livello != ancoraggio.LivelloComponente || !reflect.DeepEqual(c110.RadiciRaggiungibili, []string{t1}) ||
		!reflect.DeepEqual(c110.Padri, []string{r100("#1")}) || !reflect.DeepEqual(c110.Percorsi, [][]string{{t1, r100("#1"), r100("#2")}}) ||
		!reflect.DeepEqual(c110.Archi, []ancoraggio.ArcoPercorso{{Padre: r100("#1"), Figlio: r100("#2"), Quantita: &uno, Occorrenze: []string{"#21"}, Origine: ancoraggio.OrigineArcoFatti}}) ||
		!reflect.DeepEqual(c110.RadiciDelFile, []string{ancoraggio.RifNodo(shaPezzo110, "#1")}) || c110.Motivo != ancoraggio.MotivoCandidatoBaseDelNodo {
		t.Errorf("il candidato del pezzo di un solo assieme: %+v", c110)
	}

	condiviso := ancoraggio.RifIdentita("acme-strutture", "7120112", "", "")
	for _, id := range []uuid.UUID{idPezzo112, idPDF112} {
		a := ancoraggioDi(t, e, id)
		if a.Associazione != ancoraggio.AssociazioneCandidatoUnico || a.Collocazione != ancoraggio.CollocazioneFiglio {
			t.Fatalf("%s: il pezzo in comune: %+v", id, a)
		}
		c := a.Candidati[0]
		if c.Target != condiviso || !reflect.DeepEqual(c.RadiciRaggiungibili, []string{t1, t2}) || !reflect.DeepEqual(c.Padri, []string{r100("#1"), r103("#1")}) ||
			!reflect.DeepEqual(c.Percorsi, [][]string{{t1, r100("#1"), r100("#4")}, {t2, r103("#1"), r103("#4")}}) || len(c.Posizioni) != 2 {
			t.Errorf("%s: FIGLIO-CONDIVISO, un candidato con due radici: %+v", id, c)
		}
		archi := []ancoraggio.ArcoPercorso{
			{Padre: r100("#1"), Figlio: r100("#4"), Quantita: &due, Occorrenze: []string{"#23", "#24"}, Origine: ancoraggio.OrigineArcoFatti},
			{Padre: r103("#1"), Figlio: r103("#4"), Quantita: &uno, Origine: ancoraggio.OrigineArcoFatti},
		}
		if !reflect.DeepEqual(c.Archi, archi) {
			t.Errorf("%s: archi, quantità e percorrenze:\n%+v\nattesi:\n%+v", id, c.Archi, archi)
		}
	}
	if a := ancoraggioDi(t, e, idPDF122); a.Collocazione != ancoraggio.CollocazioneFiglio || !reflect.DeepEqual(targetDei(a), []string{r103("#2")}) ||
		!reflect.DeepEqual(a.Candidati[0].RadiciRaggiungibili, []string{t2}) {
		t.Errorf("il figlio non condiviso: %+v", a)
	}
	for _, id := range []uuid.UUID{idAssieme102, idPDF120} {
		if a := ancoraggioDi(t, e, id); a.Associazione != ancoraggio.AssociazioneNessunCandidato || a.Collocazione != ancoraggio.CollocazioneFuoriRichiesta || len(a.Motivi) != 0 {
			t.Errorf("%s: fuori richiesta atteso, con il grafo dei target completo e i pezzi «solo parti» che non contano: %+v", id, a)
		}
	}

	s100, s103 := strutturaDiE(t, e, t1, idAssieme100), strutturaDiE(t, e, t2, idAssieme103)
	for _, c := range []struct {
		s     ancoraggio.StrutturaProdotto
		rif   string
		senza bool
	}{
		{s100, r100("#1"), false}, {s100, r100("#2"), false}, {s100, r100("#3"), true}, {s100, r100("#4"), false}, {s100, r100("#5"), true},
		{s103, r103("#1"), false}, {s103, r103("#2"), false}, {s103, r103("#3"), true}, {s103, r103("#4"), false},
	} {
		if n := nodoC(t, c.s, c.rif); n.SenzaFile != c.senza {
			t.Errorf("nodo %s: SenzaFile %v, atteso %v", c.rif, n.SenzaFile, c.senza)
		}
	}
}

// ---- A1c-L1-08: non determinabile ----

// TestL108NonDeterminabile (A1c-L1-08; 6.4.5 regola 3; v3 §2 r.167; E-20): senza candidati, mai fuori_richiesta quando
// lo scenario non è noto e completo, ma non_determinabile con il motivo: la struttura di un target con il grafo
// incompleto, un target senza struttura, nessun target, un file senza letture d'identità. I file «solo parti» dei figli
// non rendono incompleto lo scenario.
func TestL108NonDeterminabile(t *testing.T) {
	m := motoreAncoraggi(t)
	scansione := pdfF(t, m, uidS(0x721), "scansione.pdf", shaS("f"), nil)
	if scansione.Disponibilita != ancoraggio.DisponibilitaSenzaTesto {
		t.Fatalf("la scansione deve essere senza testo: %q", scansione.Disponibilita)
	}

	t.Run("grafo incompleto", func(t *testing.T) {
		file, strutture := scenaFileACME(t, m, "lettura troncata: nodi")
		e := ancoraA(t, file, targetScenaACME(t, m), ancoraggio.ContestoStrutturale{Strutture: strutture})
		for _, id := range []uuid.UUID{idAssieme102, idPDF120} {
			if a := ancoraggioDi(t, e, id); a.Collocazione != ancoraggio.CollocazioneNonDeterminabile ||
				!reflect.DeepEqual(a.Motivi, []string{ancoraggio.MotivoAncoraggioGrafoIncompleto}) {
				t.Errorf("%s: non determinabile con il grafo incompleto atteso: %+v", id, a)
			}
		}
		if a := ancoraggioDi(t, e, idPezzo110); a.Collocazione != ancoraggio.CollocazioneFiglio {
			t.Errorf("con il grafo incompleto i candidati restano: %+v", a)
		}
	})
	t.Run("target senza struttura", func(t *testing.T) {
		file, strutture := scenaFileACME(t, m, "")
		target := append(targetScenaACME(t, m), targetS(t, m, "scenario:caso-acme:3", ancoraggio.AutoritaScenario, "P7120104"))
		e := ancoraA(t, file, target, ancoraggio.ContestoStrutturale{Strutture: strutture})
		if a := ancoraggioDi(t, e, idPDF120); a.Collocazione != ancoraggio.CollocazioneNonDeterminabile ||
			!reflect.DeepEqual(a.Motivi, []string{ancoraggio.MotivoAncoraggioTargetSenzaStruttura}) {
			t.Errorf("un target senza struttura: %+v", a)
		}
	})
	t.Run("target ignoti", func(t *testing.T) {
		file, strutture := scenaFileACME(t, m, "")
		e := ancoraA(t, file, nil, ancoraggio.ContestoStrutturale{Strutture: strutture})
		for _, a := range e.File {
			if a.Associazione != ancoraggio.AssociazioneNessunCandidato || a.Collocazione != ancoraggio.CollocazioneNonDeterminabile ||
				!reflect.DeepEqual(a.Motivi, []string{ancoraggio.MotivoAncoraggioTargetIgnoti}) {
				t.Errorf("%s: senza target nessun file si colloca: %+v", a.AllegatoID, a)
			}
		}
	})
	t.Run("file senza lettura", func(t *testing.T) {
		file, strutture := scenaFileACME(t, m, "")
		e := ancoraA(t, append(file, scansione), targetScenaACME(t, m), ancoraggio.ContestoStrutturale{Strutture: strutture})
		a := ancoraggioDi(t, e, scansione.AllegatoID)
		if a.Associazione != ancoraggio.AssociazioneNessunCandidato || a.Collocazione != ancoraggio.CollocazioneNonDeterminabile ||
			!reflect.DeepEqual(a.Motivi, []string{ancoraggio.MotivoAncoraggioNessunaLettura}) || a.Letture != nil ||
			a.Disponibilita != ancoraggio.DisponibilitaSenzaTesto {
			t.Errorf("un file senza lettura d'identità è non determinabile, mai fuori richiesta: %+v", a)
		}
	})
}

// ---- A1c-L1-09: il figlio condiviso a due livelli e le revisioni discordanti ----

// TestL109FiglioCondivisoARevisioni (A1c-L1-09; FIGLIO-CONDIVISO; v3 §2 r.162-163; C-21): in un DAG a due livelli il
// file del figlio sotto due padri è un candidato solo, con i due padri immediati (diversi dalla radice), i due percorsi
// e i quattro archi con quantità e occorrenze; il nipote ha i due percorsi. La stessa base raggiungibile con revisioni
// lette diverse dà candidati distinti, ambiguo, e la revisione discordante resta visibile; una formazione STEP non è
// una revisione e non divide (D1).
func TestL109FiglioCondivisoARevisioni(t *testing.T) {
	t.Run("due livelli", func(t *testing.T) {
		m := motoreAncoraggi(t)
		sha := shaS("a")
		f, s := stepF(t, m, uidS(0x730), sha, []nodoS{{"#1", "7120200", "GRUPPO"}, {"#2", "7120210", "LATO DX"}, {"#3", "7120220", "LATO SX"}, {"#4", "7120230", "PERNO"}, {"#6", "7120240", "RONDELLA"}},
			[]arcoS{{"#1", "#2", 1, nil}, {"#1", "#3", 1, nil}, {"#2", "#4", 2, []string{"#13", "#14"}}, {"#3", "#4", 1, []string{"#15"}}, {"#4", "#6", 3, nil}}, "")
		tg := targetS(t, m, "componente:"+uidS(0x731).String(), ancoraggio.AutoritaConfermata, "7120200")
		perno, rondella := cartiglio(t, m, 0x732, "7120230"), cartiglio(t, m, 0x733, "7120240")
		e := ancoraA(t, []ancoraggio.FileInterpretato{f, perno, rondella}, []ancoraggio.ProdottoRichiesto{tg}, ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{s}})
		r := func(k string) string { return ancoraggio.RifNodo(sha, k) }
		uno, due, tre := 1, 2, 3
		a := ancoraggioDi(t, e, perno.AllegatoID)
		if a.Associazione != ancoraggio.AssociazioneCandidatoUnico || a.Collocazione != ancoraggio.CollocazioneFiglio {
			t.Fatalf("il figlio condiviso: %+v", a)
		}
		c := a.Candidati[0]
		archi := []ancoraggio.ArcoPercorso{
			{Padre: r("#1"), Figlio: r("#2"), Quantita: &uno, Origine: ancoraggio.OrigineArcoFatti},
			{Padre: r("#1"), Figlio: r("#3"), Quantita: &uno, Origine: ancoraggio.OrigineArcoFatti},
			{Padre: r("#2"), Figlio: r("#4"), Quantita: &due, Occorrenze: []string{"#13", "#14"}, Origine: ancoraggio.OrigineArcoFatti},
			{Padre: r("#3"), Figlio: r("#4"), Quantita: &uno, Occorrenze: []string{"#15"}, Origine: ancoraggio.OrigineArcoFatti},
		}
		if c.Target != r("#4") || !reflect.DeepEqual(c.Padri, []string{r("#2"), r("#3")}) || !reflect.DeepEqual(c.RadiciRaggiungibili, []string{tg.Rif}) ||
			!reflect.DeepEqual(c.Percorsi, [][]string{{tg.Rif, r("#1"), r("#2"), r("#4")}, {tg.Rif, r("#1"), r("#3"), r("#4")}}) || !reflect.DeepEqual(c.Archi, archi) ||
			c.Autorita != ancoraggio.AutoritaConfermata {
			t.Errorf("padri immediati diversi dalle radici, percorsi e archi:\n%+v", c)
		}
		n := ancoraggioDi(t, e, rondella.AllegatoID).Candidati[0]
		if !reflect.DeepEqual(n.Padri, []string{r("#4")}) || len(n.Percorsi) != 2 || len(n.Archi) != 5 || *n.Archi[4].Quantita != tre {
			t.Errorf("il nipote: %+v", n)
		}
	})
	t.Run("revisioni discordanti", func(t *testing.T) {
		m := motoreCatena(t)
		sha := shaS("b")
		s := stepR(t, m, uidS(0x734), "telaio.stp", sha, []nodoR{{"#1", "7120100A", "7120100A", ""}, {"#2", "7120105A_1", "PIASTRA", ""}, {"#3", "7120105A_2", "PIASTRA", ""},
			{"#4", "7120106A", "PERNO", "1"}, {"#5", "7120106A", "PERNO", "B"}}, []arcoS{{"#1", "#2", 1, nil}, {"#1", "#3", 1, nil}, {"#1", "#4", 1, nil}, {"#1", "#5", 1, nil}})
		tg := targetC(t, m, "identificativo:7120100A", ancoraggio.AutoritaConfermata, "7120100A", nil)
		piastra := pdfF(t, m, uidS(0x735), "piastra.pdf", shaS("c"), []campoP{{"codice", "7120105A1"}})
		perno := pdfF(t, m, uidS(0x736), "perno.pdf", shaS("d"), []campoP{{"codice", "7120106A1"}})
		altroMarcatore := pdfF(t, m, uidS(0x737), "piastra-b.pdf", shaS("e"), []campoP{{"codice", "7120105B1"}})
		e := ancoraA(t, []ancoraggio.FileInterpretato{piastra, perno, altroMarcatore}, []ancoraggio.ProdottoRichiesto{tg},
			ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{s}, CodiciProposti: codiciProposti(m, s)})
		r := func(k string) string { return ancoraggio.RifNodo(sha, k) }
		a := ancoraggioDi(t, e, piastra.AllegatoID)
		if a.Associazione != ancoraggio.AssociazioneAmbiguo || !reflect.DeepEqual(a.Motivi, []string{ancoraggio.MotivoAncoraggioRevisioniDiscordanti}) ||
			!reflect.DeepEqual(targetDei(a), []string{r("#2"), r("#3")}) {
			t.Fatalf("revisioni lette diverse: candidati distinti, ambiguo: %+v", a)
		}
		if len(a.Candidati[0].Conflitti) != 0 || !reflect.DeepEqual(a.Candidati[1].Conflitti, []string{ancoraggio.DimensioneRevisione}) ||
			dimensione(a.Candidati[1], ancoraggio.DimensioneRevisione, r("#3")) != motorea.CompatibilitaDiscordante {
			t.Errorf("la revisione discordante resta visibile, e il candidato con la stessa revisione viene prima, senza essere scelto: %+v", a.Candidati)
		}
		p := ancoraggioDi(t, e, perno.AllegatoID)
		if p.Associazione != ancoraggio.AssociazioneCandidatoUnico || len(p.Candidati[0].Posizioni) != 2 ||
			p.Candidati[0].Target != ancoraggio.RifIdentita("acme-catena", "7120106", "A", "") {
			t.Errorf("due formazioni diverse non dividono la stessa identità (D1): %+v", p)
		}
		if b := ancoraggioDi(t, e, altroMarcatore.AllegatoID); b.Associazione != ancoraggio.AssociazioneNessunCandidato || b.Collocazione != ancoraggio.CollocazioneFuoriRichiesta {
			t.Errorf("la stessa base con un altro marcatore non è la stessa identità (R86): %+v", b)
		}
	})
	t.Run("il candidato a livello prodotto con il marcatore (T-B4-06 rivisto)", func(t *testing.T) {
		m := motoreCatena(t)
		sha := shaS("b")
		s := stepR(t, m, uidS(0x738), "telaio.stp", sha, []nodoR{{"#1", "7120100A", "7120100A", ""}}, nil)
		ctx := ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{s}, CodiciProposti: codiciProposti(m, s)}
		stesso := pdfF(t, m, uidS(0x739), "telaio-a.pdf", shaS("c"), []campoP{{"codice", "7120100A2"}})
		altro := pdfF(t, m, uidS(0x73a), "telaio-b.pdf", shaS("d"), []campoP{{"codice", "7120100B2"}})
		conA := targetC(t, m, "identificativo:7120100A", ancoraggio.AutoritaConfermata, "7120100A", nil)
		senza := conA
		senza.Marcatore = ""
		for _, c := range []struct {
			nome          string
			tg            ancoraggio.ProdottoRichiesto
			stesso, altro bool
		}{
			{"il target con il marcatore A", conA, true, false},
			{"il target senza marcatore: un marcatore da una parte sola è compatibile", senza, true, true},
		} {
			e := ancoraA(t, []ancoraggio.FileInterpretato{stesso, altro}, []ancoraggio.ProdottoRichiesto{c.tg}, ctx)
			for _, x := range []struct {
				f      ancoraggio.FileInterpretato
				atteso bool
			}{{stesso, c.stesso}, {altro, c.altro}} {
				a := ancoraggioDi(t, e, x.f.AllegatoID)
				prodotto := false
				for _, k := range a.Candidati {
					prodotto = prodotto || (k.Livello == ancoraggio.LivelloProdotto && k.Target == c.tg.Rif)
				}
				if prodotto != x.atteso {
					t.Errorf("%s, file %s: candidato a livello prodotto %v, atteso %v: %+v", c.nome, x.f.AllegatoID, prodotto, x.atteso, a)
				}
			}
		}
	})
}

// ---- A1c-L1-10: «SPECCHIATO DI» ----

// TestL110IlTestoNonVinceSulCartiglio (A1c-L1-10; v3 §2 r.168; P1 §10.1; C-39): il testo libero «SPECCHIATO DI …» del
// PDF è una menzione e non vince sul campo codice del cartiglio, nei due versi; il particolare simile non dà identità.
func TestL110IlTestoNonVinceSulCartiglio(t *testing.T) {
	m := motoreAncoraggi(t)
	file, strutture := scenaFileACME(t, m, "")
	ctx := ancoraggio.ContestoStrutturale{Strutture: strutture}
	target := targetScenaACME(t, m)
	r100, r103 := ancoraggio.RifNodo(shaAssieme100, "#2"), ancoraggio.RifNodo(shaAssieme103, "#2")
	for _, c := range []struct {
		cartiglio, testo, atteso string
	}{{"7120110", "SPECCHIATO DI 7120122", r100}, {"7120122", "SPECCHIATO DI 7120110", r103}} {
		pdf := pdfF(t, m, uidS(0x741), "disegno.pdf", shaS("9"), []campoP{{"codice", c.cartiglio}}, c.testo)
		menzioni := 0
		for _, l := range pdf.Interpretazione.Letture {
			if l.Funzione == motorea.FunzMenzione && strings.Contains(c.testo, l.Forma.Base.Normalizzata) {
				menzioni++
			}
		}
		if menzioni != 1 {
			t.Fatalf("il testo libero deve dare la sua menzione: %+v", pdf.Interpretazione.Letture)
		}
		e := ancoraA(t, append(append([]ancoraggio.FileInterpretato(nil), file...), pdf), target, ctx)
		a := ancoraggioDi(t, e, pdf.AllegatoID)
		if a.Associazione != ancoraggio.AssociazioneCandidatoUnico || !reflect.DeepEqual(targetDei(a), []string{c.atteso}) || len(a.Letture) != 1 {
			t.Errorf("cartiglio %s, testo %q: solo il candidato del cartiglio: %+v", c.cartiglio, c.testo, a)
		}
	}
	simile := pdfF(t, m, uidS(0x742), "disegno-simile.pdf", shaS("a"), []campoP{{"codice", "7120110"}, {"particolare_simile", "7120122"}})
	e := ancoraA(t, []ancoraggio.FileInterpretato{simile}, target, ctx)
	if a := ancoraggioDi(t, e, simile.AllegatoID); !reflect.DeepEqual(targetDei(a), []string{r100}) {
		t.Errorf("il particolare simile non dà identità: %+v", a)
	}
}

// ---- A1c-L1-11: HASH-CONFLITTO ----

// TestL111HashConflitto (A1c-L1-11; 6.4.5 regola 7; R64 A, T-B0-35): lo stesso contenuto in due allegati, con due nomi
// che dicono identità diverse: due ancoraggi, mai fusi. Il primo ha nome e cartiglio d'accordo; il secondo ha due
// identità discordanti: resta discordante, con i candidati di tutte e due, e il cartiglio non sposta l'ancoraggio da
// solo. Lo stesso STEP in due allegati: due ancoraggi, lo stesso candidato, due strutture.
func TestL111HashConflitto(t *testing.T) {
	m := motoreAncoraggi(t)
	file, strutture := scenaFileACME(t, m, "")
	sha := shaS("9")
	giusto := pdfF(t, m, uidS(0x751), "7120110.pdf", sha, []campoP{{"codice", "7120110"}})
	sbagliato := pdfF(t, m, uidS(0x752), "7120122.pdf", sha, []campoP{{"codice", "7120110"}})
	copia, sCopia := stepF(t, m, uidS(0x753), shaPezzo110, []nodoS{{"#1", "7120110", "PIASTRA"}}, nil, motivoSoloParti)
	e := ancoraA(t, append(append([]ancoraggio.FileInterpretato(nil), file...), giusto, sbagliato, copia), targetScenaACME(t, m),
		ancoraggio.ContestoStrutturale{Strutture: append(append([]ancoraggio.StrutturaFile(nil), strutture...), sCopia)})
	r110, r122 := ancoraggio.RifNodo(shaAssieme100, "#2"), ancoraggio.RifNodo(shaAssieme103, "#2")
	if a := ancoraggioDi(t, e, giusto.AllegatoID); a.Associazione != ancoraggio.AssociazioneCandidatoUnico || !reflect.DeepEqual(targetDei(a), []string{r110}) ||
		len(a.Letture) != 2 || len(a.Candidati[0].Letture) != 2 {
		t.Errorf("nome e cartiglio d'accordo, un candidato con le due letture: %+v", a)
	}
	b := ancoraggioDi(t, e, sbagliato.AllegatoID)
	if b.Associazione != ancoraggio.AssociazioneDiscordante || b.Collocazione != ancoraggio.CollocazioneFiglio ||
		!reflect.DeepEqual(b.Motivi, []string{ancoraggio.MotivoAncoraggioIdentitaDiscordanti}) || !reflect.DeepEqual(ordinateS(targetDei(b)), []string{r110, r122}) {
		t.Errorf("identità discordanti nello stesso file: discordante, con tutti e due i candidati (R64 A): %+v", b)
	}
	p, q := ancoraggioDi(t, e, idPezzo110), ancoraggioDi(t, e, copia.AllegatoID)
	if p.Collocazione != ancoraggio.CollocazioneFiglio || q.Collocazione != ancoraggio.CollocazioneFiglio || p.Candidati[0].Target != q.Candidati[0].Target {
		t.Errorf("lo stesso STEP in due allegati: due ancoraggi, lo stesso candidato: %+v %+v", p, q)
	}
}

// ---- A1c-L1-12: A-C11 ----

// TestL112StesseInterpretazioniAltriTarget (A1c-L1-12; A-C11; P1 §12): le stesse interpretazioni con due insiemi di
// target: HashIngresso uguale (le interpretazioni sono identiche), HashTarget, esiti e Impronta diversi.
func TestL112StesseInterpretazioniAltriTarget(t *testing.T) {
	m := motoreAncoraggi(t)
	file, strutture := scenaFileACME(t, m, "")
	ctx := ancoraggio.ContestoStrutturale{Strutture: strutture}
	tutti := targetScenaACME(t, m)
	a := ancoraA(t, file, tutti, ctx)
	b := ancoraA(t, file, tutti[:1], ctx)
	if a.HashIngresso != b.HashIngresso {
		t.Errorf("le stesse interpretazioni danno HashIngresso diversi: %s %s", a.HashIngresso, b.HashIngresso)
	}
	if a.HashTarget == b.HashTarget || a.Impronta == b.Impronta || canonico(t, a.File) == canonico(t, b.File) {
		t.Error("due insiemi di target danno lo stesso HashTarget, la stessa Impronta o gli stessi ancoraggi")
	}
	if x := ancoraggioDi(t, b, idAssieme103); x.Collocazione == ancoraggio.CollocazioneRadice {
		t.Errorf("senza il secondo target il suo STEP non è radice: %+v", x)
	}
	// HashTarget copre anche il contesto (parte 1 §9.4): gli stessi target con un contesto diverso.
	c := ancoraA(t, file, tutti, ancoraggio.ContestoStrutturale{Strutture: strutture, Confermato: []ancoraggio.ComponenteDeciso{
		{ComponenteID: uidS(0x7f1), Codice: "7120133", Autorita: ancoraggio.AutoritaConfermata, Origine: ancoraggio.OrigineConfermato}}})
	if c.HashIngresso != a.HashIngresso || c.HashTarget == a.HashTarget {
		t.Errorf("un contesto diverso cambia HashTarget, non HashIngresso: %s %s / %s %s", a.HashIngresso, a.HashTarget, c.HashIngresso, c.HashTarget)
	}
}

// ---- A1c-L1-13: A-C07 ----

// TestL113FormeParzialiRadiciEBasiDiscordanti (A1c-L1-13; A-C07; P1 §7.4 r.306, §10.2 r.423; C-21, C-35; D2): una
// forma parziale compatibile con due completamenti dà due candidati, ambiguo, senza completamento inventato; un file con
// due radici dà candidati distinti; una base ripetuta che non concorda dà discordante, con i candidati di tutte e due
// le basi (nessuna scelta della prima).
func TestL113FormeParzialiRadiciEBasiDiscordanti(t *testing.T) {
	t.Run("forma parziale con due completamenti", func(t *testing.T) {
		m := motore(t, famPuntiAncoraggi())
		sha := shaS("a")
		f, s := stepF(t, m, uidS(0x761), sha, []nodoS{{"#1", "9.123.0001.1", "ASSIEME"}, {"#2", "9.123.4567.3", "PEZZO"}, {"#3", "9.123.4567.5", "PEZZO"}},
			[]arcoS{{"#1", "#2", 1, nil}, {"#1", "#3", 1, nil}}, "")
		tg := targetS(t, m, "identificativo:9.123.0001.1", ancoraggio.AutoritaConfermata, "9.123.0001.1")
		parziale := pdfF(t, m, uidS(0x762), "pezzo.pdf", shaS("b"), []campoP{{"codice", "9.123.4567"}})
		e := ancoraA(t, []ancoraggio.FileInterpretato{f, parziale}, []ancoraggio.ProdottoRichiesto{tg}, ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{s}})
		a := ancoraggioDi(t, e, parziale.AllegatoID)
		if a.Associazione != ancoraggio.AssociazioneAmbiguo || !reflect.DeepEqual(a.Motivi, []string{ancoraggio.MotivoAncoraggioCompletamentiMultipli}) ||
			!reflect.DeepEqual(targetDei(a), []string{ancoraggio.RifNodo(sha, "#2"), ancoraggio.RifNodo(sha, "#3")}) {
			t.Fatalf("una forma parziale con due completamenti: %+v", a)
		}
		for _, c := range a.Candidati {
			if dimensione(c, ancoraggio.DimensioneBase, c.Target) != motorea.CompatibilitaParziale {
				t.Errorf("la base parziale resta parziale, nessun completamento: %+v", c.Dimensioni)
			}
		}
	})
	t.Run("file con due radici", func(t *testing.T) {
		m := motoreAncoraggi(t)
		sha := shaS("c")
		id := uidS(0x763)
		f := conDisponibilita(fileS(t, m, id, "due-radici.stp", "stp", sha, fattiS(t, sha, []string{"#1", "#9"},
			[]nodoS{{"#1", "7120100", "TELAIO"}, {"#2", "7120110", "PIASTRA"}, {"#9", "7120103", "STAFFA"}}, []arcoS{{"#1", "#2", 1, nil}},
			testoS("2 radici nel file: una distinta ne ha una"))), ancoraggio.DisponibilitaDisponibile)
		s := strutturaS(t, f)
		e := ancoraA(t, []ancoraggio.FileInterpretato{f}, targetScenaACME(t, m), ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{s}})
		a := ancoraggioDi(t, e, id)
		if a.Associazione != ancoraggio.AssociazioneAmbiguo || a.Collocazione != ancoraggio.CollocazioneRadice ||
			!reflect.DeepEqual(targetDei(a), []string{"scenario:caso-acme:1", "scenario:caso-acme:2"}) ||
			!reflect.DeepEqual(a.Motivi, []string{ancoraggio.MotivoAncoraggioCandidatiMultipli}) ||
			!reflect.DeepEqual(a.Candidati[0].RadiciDelFile, []string{ancoraggio.RifNodo(sha, "#1"), ancoraggio.RifNodo(sha, "#9")}) {
			t.Errorf("due radici: due candidati distinti, nessuna scelta della prima, non discordante (C-21): %+v", a)
		}
	})
	t.Run("base ripetuta che non concorda", func(t *testing.T) {
		m := motoreAncoraggi(t)
		file, strutture := scenaFileACME(t, m, "")
		rip := pdfF(t, m, uidS(0x764), "disegno.pdf", shaS("9"), []campoP{{"codice", "7120110_R7120122"}})
		discordanti := 0
		for _, l := range rip.Interpretazione.Letture {
			if l.Forma.Stato == motorea.StatoDiscordante {
				discordanti++
			}
		}
		if discordanti != 1 {
			t.Fatalf("la forma ripetuta deve dare una lettura discordante: %+v", rip.Interpretazione.Letture)
		}
		e := ancoraA(t, append(append([]ancoraggio.FileInterpretato(nil), file...), rip), targetScenaACME(t, m), ancoraggio.ContestoStrutturale{Strutture: strutture})
		a := ancoraggioDi(t, e, rip.AllegatoID)
		if a.Associazione != ancoraggio.AssociazioneDiscordante ||
			!reflect.DeepEqual(ordinateS(targetDei(a)), []string{ancoraggio.RifNodo(shaAssieme100, "#2"), ancoraggio.RifNodo(shaAssieme103, "#2")}) {
			t.Errorf("una base ripetuta che non concorda: discordante, i candidati delle due basi: %+v", a)
		}
	})
}

// ---- lo STEP e il nome del suo file ----

// TestLoSTEPEIlNomeDelSuoFile (T-B0-35, R64 A, R86; C-21): fra la radice di uno STEP e il nome del suo file la
// discordanza si guarda solo su base e marcatore. Una radice senza revisione e un nome con la revisione non sono
// discordanti; un nome che nessuna famiglia legge nemmeno. Un nome con un altro marcatore, o con un'altra base, dà
// identità discordanti: discordante, con i candidati di tutte e due le identità e nessuna scelta.
func TestLoSTEPEIlNomeDelSuoFile(t *testing.T) {
	m := motoreCatena(t)
	nodi, archi := nodiScenaC()
	tg := targetC(t, m, "identificativo:7120100A", ancoraggio.AutoritaConfermata, "7120100A", nil)
	for _, c := range []struct {
		nome         string
		associazione ancoraggio.Associazione
		target       []string
	}{
		{"7120100A_2.stp", ancoraggio.AssociazioneCandidatoUnico, []string{tg.Rif}},
		{"telaio.stp", ancoraggio.AssociazioneCandidatoUnico, []string{tg.Rif}},
		{"7120100B_2.stp", ancoraggio.AssociazioneDiscordante, []string{tg.Rif}},
		{"7120101A_1.stp", ancoraggio.AssociazioneDiscordante, []string{tg.Rif, rC("#2")}},
	} {
		f := conDisponibilita(fileS(t, m, idFileC, c.nome, "stp", shaFileC, fattiR(t, shaFileC, nodi, archi)), ancoraggio.DisponibilitaDisponibile)
		s := strutturaS(t, f)
		e := ancoraA(t, []ancoraggio.FileInterpretato{f}, []ancoraggio.ProdottoRichiesto{tg},
			ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{s}, CodiciProposti: codiciProposti(m, s)})
		a := e.File[0]
		if a.Associazione != c.associazione || !reflect.DeepEqual(targetDei(a), c.target) {
			t.Errorf("%s: associazione %s, candidati %v; attesi %s, %v", c.nome, a.Associazione, targetDei(a), c.associazione, c.target)
		}
		if (c.associazione == ancoraggio.AssociazioneDiscordante) != contiene(a.Motivi, ancoraggio.MotivoAncoraggioIdentitaDiscordanti) {
			t.Errorf("%s: motivi %v", c.nome, a.Motivi)
		}
	}
}

// ---- un file sia fra i target sia fra i figli ----

// TestFileFraITargetEFraIFigli (6.4.5 regola 3; P1 §7.4 r.306): il pezzo 7120110, chiesto come prodotto e figlio
// dell'assieme 7120100: il suo STEP e il suo PDF hanno due candidati, uno a livello prodotto e uno a livello componente:
// ambiguo, e la collocazione non si sceglie.
func TestFileFraITargetEFraIFigli(t *testing.T) {
	m := motoreAncoraggi(t)
	file, strutture := scenaFileACME(t, m, "")
	pdf := cartiglio(t, m, 0x771, "7120110")
	target := append(targetScenaACME(t, m), targetS(t, m, "scenario:caso-acme:4", ancoraggio.AutoritaScenario, "P7120110"))
	e := ancoraA(t, append(file, pdf), target, ancoraggio.ContestoStrutturale{Strutture: strutture})
	for _, id := range []uuid.UUID{idPezzo110, pdf.AllegatoID} {
		a := ancoraggioDi(t, e, id)
		if a.Associazione != ancoraggio.AssociazioneAmbiguo || a.Collocazione != ancoraggio.CollocazioneNonDeterminabile ||
			!reflect.DeepEqual(a.Motivi, []string{ancoraggio.MotivoAncoraggioLivelliDiversi}) || len(a.Candidati) != 2 ||
			a.Candidati[0].Livello != ancoraggio.LivelloProdotto || a.Candidati[1].Livello != ancoraggio.LivelloComponente {
			t.Errorf("%s: un file fra i target e fra i figli: %+v", id, a)
		}
	}
}

// ---- PO-20: l'identità parziale accanto ----

// TestPO20CandidatoSuUnaIdentitaParziale (PO-20, la parte di B4; T-E1-03, R87): lo STEP non ha la revisione (la radice e
// un figlio leggono base e marcatore, la forma del cartiglio chiede la revisione) e arriva un 2D plausibile: la proposta
// non aspetta la nomenclatura, è candidato_unico, e porta accanto l'incertezza dell'identità (parziale, con il motivo);
// la revisione del 2D non diventa la revisione del nodo.
func TestPO20CandidatoSuUnaIdentitaParziale(t *testing.T) {
	m := motoreCatena(t)
	s := scenaC(t, m, "telaio.stp")
	tg := targetC(t, m, ancoraggio.RifComponente(idC1), ancoraggio.AutoritaConfermata, "7120100A", uuidP(idC1))
	prodotto := pdfF(t, m, uidS(0x781), "telaio.pdf", shaS("1"), []campoP{{"codice", "7120100A2"}})
	figlio := pdfF(t, m, uidS(0x782), "supporto.pdf", shaS("2"), []campoP{{"codice", "7120101A1"}})
	e := ancoraA(t, []ancoraggio.FileInterpretato{prodotto, figlio}, []ancoraggio.ProdottoRichiesto{tg},
		ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{s}, CodiciProposti: codiciProposti(m, s)})
	for _, c := range []struct {
		id      uuid.UUID
		livello string
		target  string
	}{{prodotto.AllegatoID, ancoraggio.LivelloProdotto, tg.Rif}, {figlio.AllegatoID, ancoraggio.LivelloComponente, rC("#2")}} {
		a := ancoraggioDi(t, e, c.id)
		if a.Associazione != ancoraggio.AssociazioneCandidatoUnico || len(a.Candidati) != 1 {
			t.Fatalf("%s: candidato unico atteso: %+v", c.id, a)
		}
		x := a.Candidati[0]
		if x.Livello != c.livello || x.Target != c.target || !x.IdentitaParziale ||
			!reflect.DeepEqual(x.MotiviIdentita, []string{motorea.MotivoComposizioneRevisioneNonDeterminata}) {
			t.Errorf("%s: il candidato con l'incertezza dell'identità accanto: %+v", c.id, x)
		}
	}
	n := nodoC(t, strutturaDiE(t, e, tg.Rif, idFileC), rC("#2"))
	if n.Codice.Identita.Revisione != nil || !n.Codice.Identita.Parziale || n.SenzaFile {
		t.Errorf("il nodo resta parziale, ma non è più senza file: %+v", n)
	}
}

// ---- PO-21: due fotografie ----

// TestPO21LaCompatibilitaSiRicalcola (PO-21, la parte di B4; T-E1-04, T-E1-05; emendamento E1 §4.2): due fotografie.
// (a) La correzione di una riga aperta: lo stesso nodo, lo stesso candidato; la compatibilità con il codice manuale si
// ricalcola, e il file che non è più compatibile lo dice con il conflitto. (b) La rinomina di un componente confermato:
// lo stesso componente (il Target del candidato è il componente), la compatibilità con il codice confermato si
// ricalcola e lo dice; l'abbinamento del nodo dice che la decisione non è più compatibile. Il conflitto composto è di B6.
func TestPO21LaCompatibilitaSiRicalcola(t *testing.T) {
	m := motoreCatena(t)
	s := scenaC(t, m, "telaio.stp")
	tg := targetC(t, m, "identificativo:7120100A", ancoraggio.AutoritaConfermata, "7120100A", nil)
	supporto := pdfF(t, m, uidS(0x791), "supporto.pdf", shaS("1"), []campoP{{"codice", "7120101A1"}})
	piastra := pdfF(t, m, uidS(0x792), "piastra.pdf", shaS("2"), []campoP{{"codice", "7120102A3"}})
	aperta := rigaAperta(uidS(0x793), idFileC, shaFileC, "#2")
	decisa := rigaDecisa(uidS(0x794), idFileC, shaFileC, "#3", ancoraggio.StatoRigaConfermata, uuidP(idC3), uuidP(idUtente))
	foto := func(aperta ancoraggio.RigaPropostaLegacy, codiceC3 string) ancoraggio.EsitoAncoraggi {
		t.Helper()
		return ancoraA(t, []ancoraggio.FileInterpretato{supporto, piastra}, []ancoraggio.ProdottoRichiesto{tg}, ancoraggio.ContestoStrutturale{
			Strutture: []ancoraggio.StrutturaFile{s}, CodiciProposti: codiciProposti(m, s), Proposto: []ancoraggio.RigaPropostaLegacy{aperta},
			Decise: []ancoraggio.RigaDecisaLegacy{decisa}, Confermato: []ancoraggio.ComponenteDeciso{componenteC(t, m, idC3, codiceC3, nil)}})
	}
	manuale, lm := "7120109A", letturaC(t, m, "nodo_step.id", "7120109A")
	corretta := aperta
	corretta.CodiceManuale, corretta.LetturaManuale = &manuale, &lm
	prima, dopo := foto(aperta, "7120102A"), foto(corretta, "7120108A")

	sp, sd := ancoraggioDi(t, prima, supporto.AllegatoID).Candidati[0], ancoraggioDi(t, dopo, supporto.AllegatoID).Candidati[0]
	riga := ancoraggio.RifRigaProposta(aperta.ID)
	if sp.Target != rC("#2") || sd.Target != rC("#2") || dimensione(sp, ancoraggio.DimensioneCodiceManuale, riga) != "" ||
		dimensione(sd, ancoraggio.DimensioneCodiceManuale, riga) != motorea.CompatibilitaDiscordante || !contiene(sd.Conflitti, ancoraggio.DimensioneCodiceManuale) {
		t.Errorf("(a) la riga aperta corretta: lo stesso nodo, la compatibilità con il codice manuale ricalcolata:\nprima %+v\ndopo %+v", sp, sd)
	}
	pp, pd := ancoraggioDi(t, prima, piastra.AllegatoID).Candidati[0], ancoraggioDi(t, dopo, piastra.AllegatoID).Candidati[0]
	c3 := ancoraggio.RifComponente(idC3)
	if pp.Target != c3 || pd.Target != c3 || dimensione(pp, ancoraggio.DimensioneCodiceConfermato, c3) != motorea.CompatibilitaUguale || len(pp.Conflitti) != 0 ||
		dimensione(pd, ancoraggio.DimensioneCodiceConfermato, c3) != motorea.CompatibilitaDiscordante || !contiene(pd.Conflitti, ancoraggio.DimensioneCodiceConfermato) {
		t.Errorf("(b) il componente rinominato: lo stesso componente, la compatibilità con il codice confermato ricalcolata:\nprima %+v\ndopo %+v", pp, pd)
	}
	if a := ancoraggioDi(t, dopo, piastra.AllegatoID); a.Associazione != ancoraggio.AssociazioneCandidatoUnico {
		t.Errorf("il segnale non cambia l'associazione proposta: %+v", a)
	}
	// Una rinomina che cambia solo il marcatore: la base è la stessa, ma il marcatore fa parte dell'identità (R86).
	marcatore := ancoraggioDi(t, foto(aperta, "7120102B"), piastra.AllegatoID).Candidati[0]
	if dimensione(marcatore, ancoraggio.DimensioneCodiceConfermato, c3) != motorea.CompatibilitaDiscordante {
		t.Errorf("un altro marcatore del codice confermato è discordante: %+v", marcatore.Dimensioni)
	}
	np := nodoC(t, strutturaDiE(t, prima, tg.Rif, idFileC), rC("#3"))
	nd := nodoC(t, strutturaDiE(t, dopo, tg.Rif, idFileC), rC("#3"))
	if np.AbbinamentoPerBase != nil || nd.AbbinamentoPerBase == nil || nd.AbbinamentoPerBase.MotivoDecisione != ancoraggio.MotivoAbbinamentoDecisioneNonCompatibile ||
		nd.Decisione == nil || nd.Decisione.ComponenteID != idC3 {
		t.Errorf("la decisione resta, e la perdita di compatibilità si vede: prima %+v, dopo %+v", np.AbbinamentoPerBase, nd.AbbinamentoPerBase)
	}
}

// ---- la pre-associazione (R85) ----

// TestPreAssociazioneSullaBOMDiLavoro (R85, R59 A; contratto §1.3, §1.5, la riga B4 del §7): prima della conferma della
// fonte i file si associano solo alle strutture candidate; con la fonte confermata, nello stesso calcolo, anche ai nodi
// della BOM di lavoro, e le altre strutture restano candidate. Nessuna associazione proposta diventa confermata.
func TestPreAssociazioneSullaBOMDiLavoro(t *testing.T) {
	m := motoreAncoraggi(t)
	strutture := scenaR59(t, m)
	pezzo := cartiglio(t, m, 0x7a1, "7120311")
	piastra := cartiglio(t, m, 0x7a2, "7120320")
	file := []ancoraggio.FileInterpretato{pezzo, piastra}
	prima := ancoraA(t, file, []ancoraggio.ProdottoRichiesto{targetR59(t, m, nil)}, ancoraggio.ContestoStrutturale{Strutture: strutture})
	dopo := ancoraA(t, file, []ancoraggio.ProdottoRichiesto{targetR59(t, m, fonteR59(shaR59a, "#1", &idR59a))}, ancoraggio.ContestoStrutturale{Strutture: strutture})
	stati := func(e ancoraggio.EsitoAncoraggi, id uuid.UUID) map[uuid.UUID]ancoraggio.StatoStruttura {
		out := map[uuid.UUID]ancoraggio.StatoStruttura{}
		for _, c := range ancoraggioDi(t, e, id).Candidati {
			if c.Origine != ancoraggio.OrigineProposto {
				t.Errorf("un candidato non proposto: %+v", c)
			}
			for _, p := range c.Posizioni {
				out[p.AllegatoID] = p.Stato
			}
		}
		return out
	}
	candidata, bom := ancoraggio.StatoStrutturaCandidata, ancoraggio.StatoBOMDiLavoroProposta
	if got := stati(prima, pezzo.AllegatoID); !reflect.DeepEqual(got, map[uuid.UUID]ancoraggio.StatoStruttura{idR59a: candidata, idR59c: candidata}) {
		t.Errorf("prima della conferma solo strutture candidate: %v", got)
	}
	if got := stati(dopo, pezzo.AllegatoID); !reflect.DeepEqual(got, map[uuid.UUID]ancoraggio.StatoStruttura{idR59a: bom, idR59c: candidata}) {
		t.Errorf("dopo la conferma il file sta anche sulla BOM di lavoro, l'altra struttura resta candidata: %v", got)
	}
	if got := stati(dopo, piastra.AllegatoID); !reflect.DeepEqual(got, map[uuid.UUID]ancoraggio.StatoStruttura{idR59b: candidata}) {
		t.Errorf("un file del secondo STEP resta sulla sua struttura candidata: %v", got)
	}
	for _, e := range []ancoraggio.EsitoAncoraggi{prima, dopo} {
		for _, a := range e.File {
			if a.Associazione != ancoraggio.AssociazioneAmbiguo && a.Associazione != ancoraggio.AssociazioneCandidatoUnico {
				t.Errorf("%s: associazione %q", a.AllegatoID, a.Associazione)
			}
		}
	}
	b := strutturaDiE(t, dopo, ancoraggio.RifComponente(componenteR59), idR59a)
	for _, s := range dopo.Strutture {
		if s.AllegatoID == idR59a && s.Radice == ancoraggio.RifNodo(shaR59a, "#1") {
			b = s
		}
	}
	if b.Stato != bom || nodoC(t, b, ancoraggio.RifNodo(shaR59a, "#3")).SenzaFile || !nodoC(t, b, ancoraggio.RifNodo(shaR59a, "#2")).SenzaFile {
		t.Errorf("la BOM di lavoro con i file pre-associati: %+v", b)
	}
}

// ---- uno STEP in un messaggio qualsiasi ----

// TestUnoSTEPInUnMessaggioQualsiasi (R93, T-E1-12, PO-32 nella parte di B4): uno STEP con la radice di un prodotto,
// allegato a un messaggio qualsiasi (anche fuori dal perimetro), resta un file con i suoi candidati: la pertinenza e il
// perimetro li decide valutazione (B6), che legge la fotografia.
func TestUnoSTEPInUnMessaggioQualsiasi(t *testing.T) {
	m := motoreAncoraggi(t)
	sha := shaS("9")
	id := uidS(0x7b1)
	a := fotorfq.Allegato{ID: id, MessaggioID: uidS(0x7b2), Indice: 3, NomeFile: "ACME-030P7120100 00 IN_WORK.stp", Estensione: testoS("stp"), Natura: "file",
		Origine: "outlook", Stato: "analizzato", Sha256: testoS(sha), RicevutoIl: dataStrutture}
	d, err := estrazione.DaAllegato(a, fattiS(t, sha, []string{"#1"}, []nodoS{{"#1", "7120100", "TELAIO"}, {"#2", "7120110", "PIASTRA"}}, []arcoS{{"#1", "#2", 1, nil}}, testoS("")), nil)
	if err != nil {
		t.Fatal(err)
	}
	r, err := m.Interpreta(d, evidenze.UsoSconosciuto(d.BundleID))
	if err != nil {
		t.Fatal(err)
	}
	f := ancoraggio.FileInterpretato{AllegatoID: id, Documento: d, Interpretazione: r, Disponibilita: ancoraggio.DisponibilitaDisponibile}
	e := ancoraA(t, []ancoraggio.FileInterpretato{f}, targetScenaACME(t, m)[:1], ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{strutturaS(t, f)}})
	if x := ancoraggioDi(t, e, id); x.Collocazione != ancoraggio.CollocazioneRadice || x.Associazione != ancoraggio.AssociazioneCandidatoUnico {
		t.Errorf("lo STEP resta un file con i suoi candidati: %+v", x)
	}
}

// ---- l'abbinamento per base (emendamento E1 §4.2, punto 2) ----

// TestAbbinamentoPerBase (emendamento E1 §4.2, riga «decisione del nodo»; workflow, passo 6; T-B4-12, T-B4-26,
// T-B4-27; T-B4-30): senza la decisione per UUID il nodo mostra, come proposta e mai come decisione, il componente
// confermato con la stessa identità (namespace, base completa, marcatore compatibile: anche scritto da una parte sola);
// con più componenti della stessa base tutti, senza scelta; nessun abbinamento senza componenti di quell'identità (due
// marcatori scritti e diversi non lo sono). La decisione per UUID vince sull'abbinamento; se la base del componente
// deciso non è più compatibile, o non si può verificare (codice non letto, letto in un altro namespace), lo dice il
// motivo della decisione; un marcatore scritto da una parte sola resta compatibile. Una riga decisa con un componente
// che non è fra i confermati: decisione_non_verificabile con quel componente, e l'abbinamento accanto con il suo motivo.
// Un nodo scartato non riceve l'abbinamento.
func TestAbbinamentoPerBase(t *testing.T) {
	m := motoreCatena(t)
	s := scenaC(t, m, "telaio.stp")
	tg := targetC(t, m, "identificativo:7120100A", ancoraggio.AutoritaConfermata, "7120100A", nil)
	c4, c9 := uidS(0x614), uidS(0x619)
	decisa := rigaDecisa(uidS(0x7c1), idFileC, shaFileC, "#3", ancoraggio.StatoRigaConfermata, uuidP(idC3), uuidP(idUtente))
	archiviato := rigaDecisa(uidS(0x7c3), idFileC, shaFileC, "#3", ancoraggio.StatoRigaConfermata, uuidP(c9), uuidP(idUtente))
	scartata := rigaDecisa(uidS(0x7c2), idFileC, shaFileC, "#3", ancoraggio.StatoRigaScartata, nil, uuidP(idUtente))
	nodo := func(componenti []ancoraggio.ComponenteDeciso, decise ...ancoraggio.RigaDecisaLegacy) ancoraggio.NodoProposto {
		t.Helper()
		ctx := ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{s}, CodiciProposti: codiciProposti(m, s), Confermato: componenti, Decise: decise}
		return nodoC(t, proponiC(t, []ancoraggio.ProdottoRichiesto{tg}, ctx)[0], rC("#3"))
	}
	conLettura := func(f func(*motorea.LetturaForma)) ancoraggio.ComponenteDeciso {
		k := componenteC(t, m, idC3, "7120102A", nil)
		if f == nil {
			k.Lettura = nil
			return k
		}
		f(k.Lettura)
		return k
	}
	senzaMarcatore := conLettura(func(l *motorea.LetturaForma) { l.Marcatore = nil })
	altroNamespace := conLettura(func(l *motorea.LetturaForma) { l.Namespace = "acme-catena-altro" })
	senzaLettura := conLettura(nil)
	c3 := componenteC(t, m, idC3, "7120102A", nil)
	for _, c := range []struct {
		nome       string
		componenti []ancoraggio.ComponenteDeciso
		decise     []ancoraggio.RigaDecisaLegacy
		decisione  bool
		atteso     *ancoraggio.AbbinamentoPerBase
	}{
		{"abbinamento unico", []ancoraggio.ComponenteDeciso{c3}, nil, false,
			&ancoraggio.AbbinamentoPerBase{Componenti: []uuid.UUID{idC3}, Motivo: ancoraggio.MotivoAbbinamentoPerBase}},
		{"più componenti della stessa base", []ancoraggio.ComponenteDeciso{componenteC(t, m, c4, "7120102A", nil), c3}, nil, false,
			&ancoraggio.AbbinamentoPerBase{Componenti: []uuid.UUID{idC3, c4}, Motivo: ancoraggio.MotivoAbbinamentoComponentiStessaBase}},
		{"il marcatore scritto da una parte sola è compatibile", []ancoraggio.ComponenteDeciso{senzaMarcatore}, nil, false,
			&ancoraggio.AbbinamentoPerBase{Componenti: []uuid.UUID{idC3}, Motivo: ancoraggio.MotivoAbbinamentoPerBase}},
		{"nessun abbinamento", []ancoraggio.ComponenteDeciso{componenteC(t, m, idC3, "7120199A", nil)}, nil, false, nil},
		{"un altro marcatore non è la stessa identità", []ancoraggio.ComponenteDeciso{componenteC(t, m, idC3, "7120102B", nil)}, nil, false, nil},
		{"la decisione per UUID vince", []ancoraggio.ComponenteDeciso{c3, componenteC(t, m, c4, "7120102A", nil)}, []ancoraggio.RigaDecisaLegacy{decisa}, true, nil},
		{"la decisione con il marcatore scritto da una parte sola", []ancoraggio.ComponenteDeciso{senzaMarcatore}, []ancoraggio.RigaDecisaLegacy{decisa}, true, nil},
		{"la decisione non è più compatibile", []ancoraggio.ComponenteDeciso{componenteC(t, m, idC3, "7120108A", nil)}, []ancoraggio.RigaDecisaLegacy{decisa}, true,
			&ancoraggio.AbbinamentoPerBase{Decisione: uuidP(idC3), MotivoDecisione: ancoraggio.MotivoAbbinamentoDecisioneNonCompatibile, Compatibilita: motorea.CompatibilitaDiscordante}},
		{"la decisione con un altro marcatore", []ancoraggio.ComponenteDeciso{componenteC(t, m, idC3, "7120102B", nil)}, []ancoraggio.RigaDecisaLegacy{decisa}, true,
			&ancoraggio.AbbinamentoPerBase{Decisione: uuidP(idC3), MotivoDecisione: ancoraggio.MotivoAbbinamentoDecisioneNonCompatibile, Compatibilita: motorea.CompatibilitaDiscordante}},
		{"la decisione con il codice non letto", []ancoraggio.ComponenteDeciso{senzaLettura}, []ancoraggio.RigaDecisaLegacy{decisa}, true,
			&ancoraggio.AbbinamentoPerBase{Decisione: uuidP(idC3), MotivoDecisione: ancoraggio.MotivoAbbinamentoDecisioneNonVerificabile, Compatibilita: motorea.CompatibilitaNonDeterminabile}},
		{"la decisione letta in un altro namespace", []ancoraggio.ComponenteDeciso{altroNamespace}, []ancoraggio.RigaDecisaLegacy{decisa}, true,
			&ancoraggio.AbbinamentoPerBase{Decisione: uuidP(idC3), MotivoDecisione: ancoraggio.MotivoAbbinamentoDecisioneNonVerificabile, Compatibilita: motorea.CompatibilitaNonDeterminabile}},
		{"la riga decisa con un componente fuori dal contesto, e l'abbinamento accanto", []ancoraggio.ComponenteDeciso{c3}, []ancoraggio.RigaDecisaLegacy{archiviato}, false,
			&ancoraggio.AbbinamentoPerBase{Componenti: []uuid.UUID{idC3}, Motivo: ancoraggio.MotivoAbbinamentoAccantoAllaRigaDecisa, Decisione: uuidP(c9),
				MotivoDecisione: ancoraggio.MotivoAbbinamentoDecisioneNonVerificabile, Compatibilita: motorea.CompatibilitaNonDeterminabile}},
		{"la riga decisa con un componente fuori dal contesto, senza abbinamento", nil, []ancoraggio.RigaDecisaLegacy{archiviato}, false,
			&ancoraggio.AbbinamentoPerBase{Decisione: uuidP(c9), MotivoDecisione: ancoraggio.MotivoAbbinamentoDecisioneNonVerificabile, Compatibilita: motorea.CompatibilitaNonDeterminabile}},
		{"il nodo scartato non riceve l'abbinamento", []ancoraggio.ComponenteDeciso{c3}, []ancoraggio.RigaDecisaLegacy{scartata}, false,
			&ancoraggio.AbbinamentoPerBase{Motivo: ancoraggio.MotivoNodoScartato}},
	} {
		t.Run(c.nome, func(t *testing.T) {
			n := nodo(c.componenti, c.decise...)
			if (n.Decisione != nil) != c.decisione {
				t.Fatalf("decisione %+v: l'abbinamento non è mai una decisione", n.Decisione)
			}
			if !reflect.DeepEqual(n.AbbinamentoPerBase, c.atteso) {
				t.Errorf("abbinamento %+v, atteso %+v", n.AbbinamentoPerBase, c.atteso)
			}
		})
	}
}

// TestCandidatoSuUnNodoScartato (T-B4-27; R105): il file con l'identità di un nodo scartato da una persona resta un
// candidato (lo scarto del nodo non è uno scarto del file), e la sua posizione sul nodo porta il motivo nodo_scartato;
// sugli altri nodi nessun motivo.
func TestCandidatoSuUnNodoScartato(t *testing.T) {
	m := motoreCatena(t)
	s := scenaC(t, m, "telaio.stp")
	tg := targetC(t, m, "identificativo:7120100A", ancoraggio.AutoritaConfermata, "7120100A", nil)
	piastra := pdfF(t, m, uidS(0x7c5), "piastra.pdf", shaS("7"), []campoP{{"codice", "7120102A3"}})
	supporto := pdfF(t, m, uidS(0x7c6), "supporto.pdf", shaS("8"), []campoP{{"codice", "7120101A1"}})
	e := ancoraA(t, []ancoraggio.FileInterpretato{piastra, supporto}, []ancoraggio.ProdottoRichiesto{tg}, ancoraggio.ContestoStrutturale{
		Strutture: []ancoraggio.StrutturaFile{s}, CodiciProposti: codiciProposti(m, s),
		Decise: []ancoraggio.RigaDecisaLegacy{rigaDecisa(uidS(0x7c7), idFileC, shaFileC, "#3", ancoraggio.StatoRigaScartata, nil, uuidP(idUtente))}})
	p := ancoraggioDi(t, e, piastra.AllegatoID)
	if p.Associazione != ancoraggio.AssociazioneCandidatoUnico || len(p.Candidati[0].Posizioni) != 1 || p.Candidati[0].Posizioni[0].Motivo != ancoraggio.MotivoNodoScartato {
		t.Errorf("il candidato sul nodo scartato, con il motivo: %+v", p)
	}
	if q := ancoraggioDi(t, e, supporto.AllegatoID); q.Candidati[0].Posizioni[0].Motivo != "" {
		t.Errorf("su un nodo non scartato nessun motivo: %+v", q.Candidati[0].Posizioni)
	}
}

// ---- la regola unica del marcatore, la radice scelta, il nodo scartato (T-B4-25…T-B4-30) ----

// senzaMarcatore: il file con le letture del selettore dato senza il marcatore, come le darebbe una forma che non lo
// scrive (T-B4-30: un marcatore scritto da una parte sola è compatibile).
func senzaMarcatore(f ancoraggio.FileInterpretato, contesto evidenze.Contesto) ancoraggio.FileInterpretato {
	f.Interpretazione.Letture = append([]motorea.LetturaCodice(nil), f.Interpretazione.Letture...)
	for i := range f.Interpretazione.Letture {
		if f.Interpretazione.Letture[i].Forma.Selettore.Contesto == contesto {
			f.Interpretazione.Letture[i].Forma.Marcatore = nil
		}
	}
	return f
}

// TestLaRegolaUnicaDelMarcatore (T-B4-06 rivisto, T-B4-30; R86): il marcatore è compatibile se è uguale o se è
// scritto da una parte sola, discordante se è scritto da tutte e due le parti ed è diverso, in tutti i punti:
//   - a livello prodotto, con ProdottoRichiesto.Marcatore e la dimensione del marcatore;
//   - a livello componente, con un file che non scrive il marcatore;
//   - fra il nome dello STEP e la radice, e fra il nome e il cartiglio dello stesso file (T-B0-35);
//   - nelle identità raggiungibili: un nodo senza marcatore sta con l'unico marcatore scritto della sua base, e con due
//     marcatori scritti sta a sé;
//   - nella compatibilità con il codice confermato del nodo;
//   - nelle radici candidate: un target con la B non ha come radice candidata un nodo con la A (T-B4-30).
func TestLaRegolaUnicaDelMarcatore(t *testing.T) {
	m := motoreCatena(t)
	s := scenaC(t, m, "telaio.stp")
	ctx := ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{s}, CodiciProposti: codiciProposti(m, s)}
	conA := targetC(t, m, "identificativo:7120100A", ancoraggio.AutoritaConfermata, "7120100A", nil)
	senzaM := conA
	senzaM.Marcatore = ""
	unico := func(t *testing.T, e ancoraggio.EsitoAncoraggi, id uuid.UUID) ancoraggio.CandidatoAncoraggio {
		t.Helper()
		a := ancoraggioDi(t, e, id)
		if a.Associazione != ancoraggio.AssociazioneCandidatoUnico {
			t.Fatalf("candidato unico atteso: %+v", a)
		}
		return a.Candidati[0]
	}

	t.Run("a livello prodotto", func(t *testing.T) {
		b := pdfF(t, m, uidS(0x821), "telaio-b.pdf", shaS("3"), []campoP{{"codice", "7120100B2"}})
		a := pdfF(t, m, uidS(0x822), "telaio-a.pdf", shaS("4"), []campoP{{"codice", "7120100A2"}})
		e := ancoraA(t, []ancoraggio.FileInterpretato{a, b}, []ancoraggio.ProdottoRichiesto{conA}, ctx)
		if x := ancoraggioDi(t, e, b.AllegatoID); x.Associazione != ancoraggio.AssociazioneNessunCandidato || x.Collocazione != ancoraggio.CollocazioneFuoriRichiesta {
			t.Errorf("il 2D con la B non è il prodotto con la A: %+v", x)
		}
		if c := unico(t, e, a.AllegatoID); dimensione(c, ancoraggio.DimensioneMarcatore, conA.Rif) != motorea.CompatibilitaUguale {
			t.Errorf("la dimensione del marcatore: %+v", c.Dimensioni)
		}
		e = ancoraA(t, []ancoraggio.FileInterpretato{b}, []ancoraggio.ProdottoRichiesto{senzaM}, ctx)
		if c := unico(t, e, b.AllegatoID); c.Livello != ancoraggio.LivelloProdotto || dimensione(c, ancoraggio.DimensioneMarcatore, conA.Rif) != motorea.CompatibilitaParziale {
			t.Errorf("il target senza marcatore: il marcatore scritto da una parte sola è compatibile: %+v", c)
		}
	})
	t.Run("a livello componente", func(t *testing.T) {
		f := senzaMarcatore(pdfF(t, m, uidS(0x831), "supporto.pdf", shaS("5"), []campoP{{"codice", "7120101A1"}}), evidenze.ContestoCartiglio)
		c := unico(t, ancoraA(t, []ancoraggio.FileInterpretato{f}, []ancoraggio.ProdottoRichiesto{conA}, ctx), f.AllegatoID)
		if c.Target != rC("#2") || dimensione(c, ancoraggio.DimensioneMarcatore, rC("#2")) != motorea.CompatibilitaParziale || len(c.Conflitti) != 0 {
			t.Errorf("un 2D che non scrive il marcatore resta candidato del nodo con la A: %+v", c)
		}
	})
	t.Run("il nome dello STEP e del PDF senza marcatore", func(t *testing.T) {
		nodi, archi := nodiScenaC()
		f := senzaMarcatore(conDisponibilita(fileS(t, m, idFileC, "7120100A_2.stp", "stp", shaFileC, fattiR(t, shaFileC, nodi, archi)), ancoraggio.DisponibilitaDisponibile),
			evidenze.ContestoNomeFile)
		e := ancoraA(t, []ancoraggio.FileInterpretato{f}, []ancoraggio.ProdottoRichiesto{conA}, ctx)
		if a := e.File[0]; a.Associazione != ancoraggio.AssociazioneCandidatoUnico {
			t.Errorf("il nome senza marcatore non è discordante con la radice con la A (T-B0-35): %+v", a)
		}
		p := senzaMarcatore(pdfF(t, m, uidS(0x832), "7120102A_3.pdf", shaS("6"), []campoP{{"codice", "7120102A3"}}), evidenze.ContestoNomeFile)
		if a := ancoraggioDi(t, ancoraA(t, []ancoraggio.FileInterpretato{p}, []ancoraggio.ProdottoRichiesto{conA}, ctx), p.AllegatoID); a.Associazione != ancoraggio.AssociazioneCandidatoUnico {
			t.Errorf("nome senza marcatore e cartiglio con la A, stessa base e revisione: non discordanti: %+v", a)
		}
	})
	t.Run("le identità raggiungibili", func(t *testing.T) {
		sha := shaS("b")
		x := stepR(t, m, uidS(0x841), "gruppo.stp", sha, []nodoR{{"#1", "7120100A", "7120100A", ""}, {"#2", "7120108A", "PERNO", ""}, {"#3", "7120108A", "PERNO", ""},
			{"#4", "7120108B", "PERNO", ""}}, []arcoS{{"#1", "#2", 1, nil}, {"#1", "#3", 1, nil}, {"#1", "#4", 1, nil}})
		togli := func(st ancoraggio.StrutturaFile, chiavi ...string) ancoraggio.StrutturaFile {
			st.Nodi = append([]ancoraggio.NodoStruttura(nil), st.Nodi...)
			for i, n := range st.Nodi {
				if contiene(chiavi, n.Chiave) {
					st.Nodi[i].Letture = append([]motorea.LetturaCodice(nil), n.Letture...)
					for j := range st.Nodi[i].Letture {
						st.Nodi[i].Letture[j].Forma.Marcatore = nil
					}
				}
			}
			return st
		}
		r := func(k string) string { return ancoraggio.RifNodo(sha, k) }
		perno := pdfF(t, m, uidS(0x842), "perno.pdf", shaS("c"), []campoP{{"codice", "7120108A1"}})
		// Senza il nodo con la B: il nodo senza marcatore sta con l'unico marcatore scritto, un candidato con due posizioni.
		solo := togli(x, "#3")
		solo.Nodi = []ancoraggio.NodoStruttura{solo.Nodi[0], solo.Nodi[1], solo.Nodi[2]}
		solo.Archi = solo.Archi[:2]
		c := unico(t, ancoraA(t, []ancoraggio.FileInterpretato{perno}, []ancoraggio.ProdottoRichiesto{conA}, ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{solo}}),
			perno.AllegatoID)
		if len(c.Posizioni) != 2 || c.Target != ancoraggio.RifIdentita("acme-catena", "7120108", "A", "") {
			t.Errorf("il nodo senza marcatore con l'unico marcatore della sua base: %+v", c)
		}
		// Con la A e la B scritte, il nodo senza marcatore sta a sé: il 2D con la A ha due candidati, mai la B.
		e := ancoraA(t, []ancoraggio.FileInterpretato{perno}, []ancoraggio.ProdottoRichiesto{conA}, ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{togli(x, "#3")}})
		if a := ancoraggioDi(t, e, perno.AllegatoID); a.Associazione != ancoraggio.AssociazioneAmbiguo || !reflect.DeepEqual(ordinateS(targetDei(a)), []string{r("#2"), r("#3")}) {
			t.Errorf("con due marcatori scritti il nodo senza marcatore sta a sé: %+v", a)
		}
	})
	t.Run("la compatibilità con il codice confermato", func(t *testing.T) {
		k := componenteC(t, m, idC3, "7120102A", nil)
		k.Lettura.Marcatore = nil
		f := pdfF(t, m, uidS(0x851), "piastra.pdf", shaS("d"), []campoP{{"codice", "7120102A3"}})
		e := ancoraA(t, []ancoraggio.FileInterpretato{f}, []ancoraggio.ProdottoRichiesto{conA}, ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{s},
			CodiciProposti: codiciProposti(m, s), Confermato: []ancoraggio.ComponenteDeciso{k},
			Decise: []ancoraggio.RigaDecisaLegacy{rigaDecisa(uidS(0x852), idFileC, shaFileC, "#3", ancoraggio.StatoRigaConfermata, uuidP(idC3), uuidP(idUtente))}})
		if c := unico(t, e, f.AllegatoID); dimensione(c, ancoraggio.DimensioneCodiceConfermato, ancoraggio.RifComponente(idC3)) != motorea.CompatibilitaUguale {
			t.Errorf("il codice confermato senza marcatore resta compatibile: %+v", c.Dimensioni)
		}
	})
	t.Run("T-B4-30 le radici candidate", func(t *testing.T) {
		conB := targetC(t, m, "identificativo:7120100B", ancoraggio.AutoritaConfermata, "7120100B", nil)
		if st, _ := proponiS(t, []ancoraggio.ProdottoRichiesto{conB}, ctx); len(st) != 0 {
			t.Errorf("un target con la B non ha come radice candidata un nodo con la A: %+v", st)
		}
		if st, _ := proponiS(t, []ancoraggio.ProdottoRichiesto{senzaM}, ctx); len(st) != 1 || st[0].Compatibilita != motorea.CompatibilitaUguale {
			t.Errorf("un target senza marcatore ha la radice con la A: %+v", st)
		}
	})
}

// TestLaRadiceSceltaSenzaLaBaseDelProdotto (T-B4-25; R76 A): con la BOM di lavoro
// sotto una radice scelta dall'operatore che non ha la base del prodotto, lo STEP della fonte e il 2D della radice sono
// candidati a livello prodotto del target, con il motivo radice_scelta e la base discordante con il target visibile
// nelle dimensioni (e uguale con la radice); il 2D di un figlio è figlio, sulla BOM di lavoro. Senza la fonte
// confermata la radice non è del prodotto, e i file non sono suoi.
func TestLaRadiceSceltaSenzaLaBaseDelProdotto(t *testing.T) {
	m := motoreAncoraggi(t)
	sha := shaS("5")
	idStep, c := uidS(0x801), uidS(0x802)
	step, s := stepF(t, m, idStep, sha, []nodoS{{"#1", "7120500", "TELAIO DIVERSO"}, {"#2", "7120510", "PIASTRA"}}, []arcoS{{"#1", "#2", 1, nil}}, "")
	tg := targetS(t, m, ancoraggio.RifComponente(c), ancoraggio.AutoritaConfermata, "7120100")
	tg.ComponenteID = &c
	tg.FonteConfermata = fonteR59(sha, "#1", &idStep)
	pdfRadice, pdfFiglio := cartiglio(t, m, 0x803, "7120500"), cartiglio(t, m, 0x804, "7120510")
	file := []ancoraggio.FileInterpretato{step, pdfRadice, pdfFiglio}
	e := ancoraA(t, file, []ancoraggio.ProdottoRichiesto{tg}, ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{s}})
	radice := ancoraggio.RifNodo(sha, "#1")
	for _, id := range []uuid.UUID{idStep, pdfRadice.AllegatoID} {
		a := ancoraggioDi(t, e, id)
		if a.Associazione != ancoraggio.AssociazioneCandidatoUnico || a.Collocazione != ancoraggio.CollocazioneRadice {
			t.Fatalf("%s: il file della radice scelta è il prodotto, mai fuori richiesta: %+v", id, a)
		}
		x := a.Candidati[0]
		if x.Target != tg.Rif || x.Livello != ancoraggio.LivelloProdotto || x.Motivo != ancoraggio.MotivoCandidatoRadiceScelta ||
			dimensione(x, ancoraggio.DimensioneBase, tg.Rif) != motorea.CompatibilitaDiscordante || !contiene(x.Conflitti, ancoraggio.DimensioneBase) ||
			dimensione(x, ancoraggio.DimensioneBase, radice) != motorea.CompatibilitaUguale || len(x.Posizioni) != 1 ||
			x.Posizioni[0].Nodo != radice || x.Posizioni[0].Stato != ancoraggio.StatoBOMDiLavoroProposta {
			t.Errorf("%s: il candidato sulla radice scelta: %+v", id, x)
		}
	}
	f := ancoraggioDi(t, e, pdfFiglio.AllegatoID)
	if f.Collocazione != ancoraggio.CollocazioneFiglio || f.Candidati[0].Posizioni[0].Stato != ancoraggio.StatoBOMDiLavoroProposta {
		t.Errorf("il figlio della radice scelta: %+v", f)
	}
	senza := tg
	senza.FonteConfermata = nil
	e = ancoraA(t, file, []ancoraggio.ProdottoRichiesto{senza}, ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{s}})
	for _, a := range e.File {
		if len(a.Candidati) != 0 {
			t.Errorf("senza la fonte confermata la radice non è del prodotto: %+v", a)
		}
	}
}

// famRuoloSolo: una famiglia ACME con un ruolo solo, con la base data (tre cifre e quattro), sui campi d'identità del
// file e sugli id dello STEP: un meccanismo (il ruolo della lettura), mai il significato di un cliente.
func famRuoloSolo(id, prefisso string, ruolo grammatica.Ruolo) grammatica.FamigliaCodice {
	sel := []string{"nome_file", "cartiglio.codice", "radice_step.id", "nodo_step.id"}
	return grammatica.FamigliaCodice{
		ID: id, Namespace: id, Ruoli: []grammatica.Ruolo{ruolo},
		Base:  base(grammatica.SegmentoBase{Nome: "codice", Pattern: prefisso + "[0-9]{4}", Identitario: true}),
		Forme: []grammatica.FormaCodice{{ID: "codice", Selettori: sel, Stato: grammatica.StatoAttiva, Completa: true, Parti: []grammatica.Parte{{Tipo: grammatica.TipoParteBase, Min: 1, Max: 1}}}},
		Esempi: []grammatica.EsempioCodice{
			esempio("e-codice", "cartiglio.codice", prefisso+"0100", false, grammatica.LetturaAttesa{Forma: "codice", Base: prefisso + "0100"}),
		},
	}
}

// TestIRuoliDellaLettura (6.4.5 regola 3; v3 §2): il candidato a livello prodotto chiede
// «prodotto» fra i ruoli candidati della lettura del file, quello a livello componente chiede «componente»: una
// famiglia con il solo ruolo componente non dà un prodotto, una con il solo ruolo prodotto non dà un componente.
func TestIRuoliDellaLettura(t *testing.T) {
	t.Run("solo componente: nessun prodotto", func(t *testing.T) {
		m := motore(t, famRuoloSolo("acme-solo-componente", "713", grammatica.RuoloComponente))
		tg := targetS(t, m, "identificativo:7130100", ancoraggio.AutoritaConfermata, "7130100")
		if tg.Namespace != "acme-solo-componente" {
			t.Fatalf("il target deve essere letto: %+v", tg)
		}
		f := cartiglio(t, m, 0x891, "7130100")
		e := ancoraA(t, []ancoraggio.FileInterpretato{f}, []ancoraggio.ProdottoRichiesto{tg}, ancoraggio.ContestoStrutturale{})
		if a := e.File[0]; len(a.Candidati) != 0 || len(a.Letture) != 1 {
			t.Errorf("una lettura senza il ruolo prodotto non è un candidato a livello prodotto: %+v", a)
		}
	})
	t.Run("solo prodotto: nessun componente", func(t *testing.T) {
		m := motore(t, famRuoloSolo("acme-solo-prodotto", "714", grammatica.RuoloProdotto))
		sha := shaS("e")
		step, s := stepF(t, m, uidS(0x892), sha, []nodoS{{"#1", "7140100", "ASSIEME"}, {"#2", "7140110", "PEZZO"}}, []arcoS{{"#1", "#2", 1, nil}}, "")
		tg := targetS(t, m, "identificativo:7140100", ancoraggio.AutoritaConfermata, "7140100")
		f := cartiglio(t, m, 0x893, "7140110")
		e := ancoraA(t, []ancoraggio.FileInterpretato{step, f}, []ancoraggio.ProdottoRichiesto{tg}, ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{s}})
		if a := ancoraggioDi(t, e, step.AllegatoID); a.Collocazione != ancoraggio.CollocazioneRadice {
			t.Fatalf("lo STEP del prodotto resta radice: %+v", a)
		}
		if a := ancoraggioDi(t, e, f.AllegatoID); len(a.Candidati) != 0 || a.Collocazione != ancoraggio.CollocazioneFuoriRichiesta {
			t.Errorf("una lettura senza il ruolo componente non è un candidato a livello componente: %+v", a)
		}
	})
}

// TestLeRegoleCheLeMutazioniNonVedevano (6.4.5 regole 3-5): una prova per ciascuna delle regole che restavano senza
// prova.
//   - Nello stesso file nome e cartiglio che non concordano solo nella revisione sono identità discordanti (T-B0-35).
//   - Un nodo senza revisione sta con l'unica revisione letta della sua identità: un candidato solo (6.4.5 regola 3).
//   - L'ordine del sostegno mette prima il candidato senza conflitti anche quando il suo Target viene dopo (T-E1-07).
//   - Gli archi del candidato portano accanto l'arco confermato fra le decisioni dei due nodi (T-14).
//   - A livello prodotto basta la base compatibile parziale (A-C07).
//     (La radice scelta la prova TestLaRadiceSceltaSenzaLaBaseDelProdotto, l'abbinamento per base TestAbbinamentoPerBase, i
//     ruoli TestIRuoliDellaLettura.)
func TestLeRegoleCheLeMutazioniNonVedevano(t *testing.T) {
	m := motoreCatena(t)
	s := scenaC(t, m, "telaio.stp")
	tg := targetC(t, m, ancoraggio.RifComponente(idC1), ancoraggio.AutoritaConfermata, "7120100A", uuidP(idC1))
	ctx := ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{s}, CodiciProposti: codiciProposti(m, s)}

	t.Run("nome e cartiglio solo nella revisione", func(t *testing.T) {
		f := pdfF(t, m, uidS(0x881), "7120102A_2.pdf", shaS("8"), []campoP{{"codice", "7120102A3"}})
		if a := ancoraA(t, []ancoraggio.FileInterpretato{f}, []ancoraggio.ProdottoRichiesto{tg}, ctx).File[0]; a.Associazione != ancoraggio.AssociazioneDiscordante {
			t.Errorf("nome con la revisione 2 e cartiglio con la 3: discordanti: %+v", a)
		}
	})
	t.Run("la revisione del gruppo e l'ordine del sostegno", func(t *testing.T) {
		sha := shaS("9")
		x := stepR(t, m, uidS(0x8a1), "telaio.stp", sha, []nodoR{{"#1", "7120100A", "7120100A", ""}, {"#2", "7120105A_1", "PIASTRA", ""}, {"#3", "7120105A_2", "PIASTRA", ""},
			{"#4", "7120107A_1", "PERNO", ""}, {"#5", "7120107A", "PERNO", ""}}, []arcoS{{"#1", "#2", 1, nil}, {"#1", "#3", 1, nil}, {"#1", "#4", 1, nil}, {"#1", "#5", 1, nil}})
		r := func(k string) string { return ancoraggio.RifNodo(sha, k) }
		piastra := pdfF(t, m, uidS(0x8a2), "piastra.pdf", shaS("a"), []campoP{{"codice", "7120105A2"}})
		perno := pdfF(t, m, uidS(0x8a3), "perno.pdf", shaS("b"), []campoP{{"codice", "7120107A1"}})
		e := ancoraA(t, []ancoraggio.FileInterpretato{piastra, perno}, []ancoraggio.ProdottoRichiesto{tg},
			ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{x}, CodiciProposti: codiciProposti(m, x)})
		if a := ancoraggioDi(t, e, piastra.AllegatoID); !reflect.DeepEqual(targetDei(a), []string{r("#3"), r("#2")}) {
			t.Errorf("il candidato con la stessa revisione prima, anche con il Target dopo (T-E1-07): %v", targetDei(a))
		}
		if a := ancoraggioDi(t, e, perno.AllegatoID); a.Associazione != ancoraggio.AssociazioneCandidatoUnico || len(a.Candidati[0].Posizioni) != 2 {
			t.Errorf("il nodo senza revisione sta con l'unica revisione letta: %+v", a)
		}
	})
	t.Run("l'arco confermato accanto", func(t *testing.T) {
		tre := 3
		f := pdfF(t, m, uidS(0x8b1), "supporto.pdf", shaS("c"), []campoP{{"codice", "7120101A1"}})
		e := ancoraA(t, []ancoraggio.FileInterpretato{f}, []ancoraggio.ProdottoRichiesto{tg}, ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{s},
			CodiciProposti: codiciProposti(m, s),
			Confermato:     []ancoraggio.ComponenteDeciso{componenteC(t, m, idC1, "7120100A", nil), componenteC(t, m, idC2, "7120101A", nil)},
			ArchiConfermati: []ancoraggio.ArcoPercorso{{Padre: ancoraggio.RifComponente(idC1), Figlio: ancoraggio.RifComponente(idC2), Quantita: &tre,
				Origine: ancoraggio.OrigineArcoConfermato}},
			Decise: []ancoraggio.RigaDecisaLegacy{
				rigaDecisa(uidS(0x8b2), idFileC, shaFileC, "#1", ancoraggio.StatoRigaConfermata, uuidP(idC1), uuidP(idUtente)),
				rigaDecisa(uidS(0x8b3), idFileC, shaFileC, "#2", ancoraggio.StatoRigaConfermata, uuidP(idC2), uuidP(idUtente)),
			}})
		c := ancoraggioDi(t, e, f.AllegatoID).Candidati[0]
		trovato := false
		for _, a := range c.Archi {
			trovato = trovato || (a.Origine == ancoraggio.OrigineArcoConfermato && a.Padre == ancoraggio.RifComponente(idC1) && a.Figlio == ancoraggio.RifComponente(idC2) &&
				a.Quantita != nil && *a.Quantita == 3)
		}
		if !trovato || c.Target != ancoraggio.RifComponente(idC2) {
			t.Errorf("l'arco confermato accanto all'arco dei fatti nel percorso: %+v", c)
		}
	})
	t.Run("la base compatibile parziale a livello prodotto", func(t *testing.T) {
		mp := motore(t, famPuntiAncoraggi())
		completo := targetS(t, mp, "identificativo:9.123.4567.3", ancoraggio.AutoritaConfermata, "9.123.4567.3")
		parziale := pdfF(t, mp, uidS(0x8c1), "assieme.pdf", shaS("d"), []campoP{{"codice", "9.123.4567"}})
		a := ancoraA(t, []ancoraggio.FileInterpretato{parziale}, []ancoraggio.ProdottoRichiesto{completo}, ancoraggio.ContestoStrutturale{}).File[0]
		if a.Associazione != ancoraggio.AssociazioneCandidatoUnico || a.Collocazione != ancoraggio.CollocazioneRadice ||
			dimensione(a.Candidati[0], ancoraggio.DimensioneBase, completo.Rif) != motorea.CompatibilitaParziale {
			t.Errorf("una forma parziale compatibile con il target è un candidato a livello prodotto: %+v", a)
		}
	})
}

// ---- A1c-L1-15: il determinismo ----

// TestL115AncoraggiDeterministici (A1c-L1-15; 6.4.5 regola 8; par.3.4.2): gli stessi ingressi in un altro ordine
// (file, target, strutture, nodi, archi, radici, componenti, righe, codici dei messaggi, e le letture dei documenti)
// danno gli stessi byte canonici e la stessa Impronta; due chiamate danno lo stesso esito.
func TestL115AncoraggiDeterministici(t *testing.T) {
	m := motoreAncoraggi(t)
	file, strutture := scenaFileACME(t, m, "")
	file = append(file, pdfF(t, m, uidS(0x7d1), "7120122.pdf", shaS("9"), []campoP{{"codice", "7120110"}}), pdfF(t, m, uidS(0x7d2), "scansione.pdf", shaS("0"), nil))
	c1 := uidS(0x7d3)
	uno := 1
	ctx := ancoraggio.ContestoStrutturale{
		Strutture:       append(strutture, scenaR59(t, m)...),
		Confermato:      []ancoraggio.ComponenteDeciso{{ComponenteID: c1, Codice: "7120300", Autorita: ancoraggio.AutoritaConfermata, Origine: ancoraggio.OrigineConfermato}},
		ArchiProposti:   []ancoraggio.ArcoPercorso{{Padre: ancoraggio.RifNodo(shaR59a, "#1"), Figlio: ancoraggio.RifNodo(shaR59a, "#2"), Quantita: &uno, Origine: ancoraggio.OrigineArcoProposto}},
		ArchiConfermati: nil,
	}
	tr := targetR59(t, m, fonteR59(shaR59a, "#1", &idR59a))
	target := append(targetScenaACME(t, m), tr, targetS(t, m, "scenario:caso-acme:4", ancoraggio.AutoritaScenario, "P7120110"))

	rov := ctx
	rov.Strutture = rovescia(ctx.Strutture)
	for i := range rov.Strutture {
		rov.Strutture[i].Nodi = rovescia(rov.Strutture[i].Nodi)
		rov.Strutture[i].Archi = rovescia(rov.Strutture[i].Archi)
		rov.Strutture[i].Radici = rovescia(rov.Strutture[i].Radici)
	}
	fileRov := rovescia(file)
	for i := range fileRov {
		fileRov[i].Interpretazione.Letture = rovescia(fileRov[i].Interpretazione.Letture)
	}
	a := ancoraA(t, file, target, ctx)
	b := ancoraA(t, fileRov, rovescia(target), rov)
	if canonico(t, a) != canonico(t, b) || a.Impronta != b.Impronta {
		t.Errorf("gli ancoraggi dipendono dall'ordine degli ingressi:\n%s\n%s", canonico(t, a), canonico(t, b))
	}
	if canonico(t, a) != canonico(t, ancoraA(t, file, target, ctx)) {
		t.Error("due chiamate con gli stessi ingressi danno esiti diversi")
	}
	collocazioni := map[ancoraggio.Collocazione]bool{}
	for _, x := range a.File {
		collocazioni[x.Collocazione] = true
	}
	if len(collocazioni) < 3 {
		t.Fatalf("la prova del determinismo ha bisogno di più collocazioni: %v", collocazioni)
	}
}

// ---- la disponibilità e gli errori di contratto ----

// TestLaDisponibilitaStaNellAncoraggio (par.3.3.7; T-B0-31; A-C09): la disponibilità che dà valutazione si porta
// nell'ancoraggio così com'è, senza cambiare le regole: un asse distinto, che entra in HashIngresso (è un ingresso del
// file, 6.4.5 regola 8). Una scansione è senza_testo.
func TestLaDisponibilitaStaNellAncoraggio(t *testing.T) {
	m := motoreAncoraggi(t)
	file, strutture := scenaFileACME(t, m, "")
	hash := map[string]bool{}
	for _, d := range []ancoraggio.Disponibilita{ancoraggio.DisponibilitaDisponibile, ancoraggio.DisponibilitaMancante, ancoraggio.DisponibilitaPendente,
		ancoraggio.DisponibilitaParziale, ancoraggio.DisponibilitaSenzaTesto, ancoraggio.DisponibilitaIlleggibile, ancoraggio.DisponibilitaErrore} {
		x := conDisponibilita(file[5], d)
		e := ancoraA(t, []ancoraggio.FileInterpretato{x}, targetScenaACME(t, m), ancoraggio.ContestoStrutturale{Strutture: strutture})
		if a := e.File[0]; a.Disponibilita != d || a.Collocazione != ancoraggio.CollocazioneFiglio {
			t.Errorf("disponibilità %q: %+v", d, a)
		}
		hash[e.HashIngresso] = true
	}
	if len(hash) != 7 {
		t.Errorf("sette disponibilità, %d HashIngresso: la disponibilità è un ingresso del file", len(hash))
	}
}

// TestProponiAncoraggiErroriDiContratto: un ingresso che viola il contratto è un errore di contratto, con il codice della
// foglia e il percorso, mai un esito: un allegato ripetuto o senza ID, un documento di un'altra fonte, un'interpretazione
// di un altro documento, una lettura su un'unità che non c'è, una disponibilità fuori elenco o vuota, la struttura di un
// altro contenuto dello stesso allegato; con un errore delle strutture insieme.
func TestProponiAncoraggiErroriDiContratto(t *testing.T) {
	m := motoreAncoraggi(t)
	file, strutture := scenaFileACME(t, m, "")
	pdf := file[5]
	ok := ancoraggio.ContestoStrutturale{Strutture: strutture}
	target := targetScenaACME(t, m)
	if _, err := ancoraggio.ProponiAncoraggi([]ancoraggio.FileInterpretato{pdf}, target, ok); err != nil {
		t.Fatalf("l'ingresso di base non è valido: %v", err)
	}
	cambia := func(f func(*ancoraggio.FileInterpretato)) []ancoraggio.FileInterpretato {
		x := pdf
		x.Interpretazione.Letture = append([]motorea.LetturaCodice(nil), pdf.Interpretazione.Letture...)
		f(&x)
		return []ancoraggio.FileInterpretato{x}
	}
	altroContenuto := file[3]
	altroContenuto.Documento.Fonte.RiferimentoFatti.Sha256 = shaS("0")
	for _, k := range []struct {
		nome, codice, percorso string
		file                   []ancoraggio.FileInterpretato
		target                 []ancoraggio.ProdottoRichiesto
	}{
		{"allegato ripetuto", evidenze.CodiceDocumentoIDRipetuto, "file[1].allegato_id", []ancoraggio.FileInterpretato{pdf, pdf}, target},
		{"allegato senza ID", evidenze.CodiceDocumentoRiferimentoPendente, "file[0].allegato_id", cambia(func(x *ancoraggio.FileInterpretato) { x.AllegatoID = uuid.Nil }), target},
		{"documento di un altro allegato", evidenze.CodiceDocumentoRiferimentoPendente, "file[0].documento.fonte",
			cambia(func(x *ancoraggio.FileInterpretato) { x.AllegatoID = uidS(0x7e1) }), target},
		{"interpretazione di un altro documento", evidenze.CodiceDocumentoRiferimentoPendente, "file[0].interpretazione.bundle_id",
			cambia(func(x *ancoraggio.FileInterpretato) { x.Interpretazione.BundleID = shaS("0") }), target},
		{"lettura su un'unità che non c'è", evidenze.CodiceDocumentoRiferimentoPendente, "file[0].interpretazione.letture[" + pdf.Interpretazione.Letture[0].ID + "]",
			cambia(func(x *ancoraggio.FileInterpretato) { x.Interpretazione.Letture[0].UnitaID = "u:inesistente" }), target},
		{"disponibilità fuori elenco", evidenze.CodiceDocumentoEnumIgnoto, "file[0].disponibilita", cambia(func(x *ancoraggio.FileInterpretato) { x.Disponibilita = "assente" }), target},
		{"disponibilità vuota", evidenze.CodiceDocumentoEnumIgnoto, "file[0].disponibilita", cambia(func(x *ancoraggio.FileInterpretato) { x.Disponibilita = "" }), target},
		{"struttura di un altro contenuto", evidenze.CodiceDocumentoRiferimentoPendente, "file[0].documento.fonte.riferimento_fatti.sha256",
			[]ancoraggio.FileInterpretato{altroContenuto}, target},
		{"con un errore delle strutture", evidenze.CodiceDocumentoEnumIgnoto, "target[0].autorita",
			cambia(func(x *ancoraggio.FileInterpretato) { x.Disponibilita = "assente" }), func() []ancoraggio.ProdottoRichiesto {
				x := append([]ancoraggio.ProdottoRichiesto(nil), target...)
				x[0].Autorita = ancoraggio.AutoritaProposta
				return x
			}()},
	} {
		t.Run(k.nome, func(t *testing.T) {
			e, err := ancoraggio.ProponiAncoraggi(k.file, k.target, ok)
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
			if k.nome == "con un errore delle strutture" && len(conCodice(ec.Diagnostiche, evidenze.CodiceDocumentoEnumIgnoto)) != 2 {
				t.Errorf("gli errori dei file e delle strutture insieme: %+v", ec.Diagnostiche)
			}
			if !reflect.DeepEqual(e, ancoraggio.EsitoAncoraggi{}) {
				t.Errorf("con l'errore un esito: %+v", e)
			}
		})
	}
}

// ---- i valori del contratto ----

// TestIValoriDegliAncoraggi (contratto §2.2; par.3.3.7; 6.4.5; T-B0-31; le letture dell'orchestratore T-B4-08…T-B4-12):
// i valori delle costanti, la forma del riferimento di un'identità e i campi dei tipi degli ancoraggi, con i tag JSON
// snake_case. Se uno cambia, questa prova si riscrive con «Riscritta per …».
//
// Riscritta per T-B4-25…T-B4-27: il motivo radice_scelta,
// PosizioneCandidato.Motivo, le due parti di AbbinamentoPerBase (Decisione, MotivoDecisione) e i loro motivi nuovi.
func TestIValoriDegliAncoraggi(t *testing.T) {
	for _, c := range []struct{ got, want string }{
		{string(ancoraggio.DisponibilitaDisponibile), "disponibile"},
		{string(ancoraggio.DisponibilitaMancante), "mancante"},
		{string(ancoraggio.DisponibilitaPendente), "pendente"},
		{string(ancoraggio.DisponibilitaParziale), "parziale"},
		{string(ancoraggio.DisponibilitaSenzaTesto), "senza_testo"},
		{string(ancoraggio.DisponibilitaIlleggibile), "illeggibile"},
		{string(ancoraggio.DisponibilitaErrore), "errore"},
		{string(ancoraggio.AssociazioneNonValutata), "non_valutata"},
		{string(ancoraggio.AssociazioneNessunCandidato), "nessun_candidato"},
		{string(ancoraggio.AssociazioneCandidatoUnico), "candidato_unico"},
		{string(ancoraggio.AssociazioneAmbiguo), "ambiguo"},
		{string(ancoraggio.AssociazioneDiscordante), "discordante"},
		{string(ancoraggio.CollocazioneRadice), "radice"},
		{string(ancoraggio.CollocazioneFiglio), "figlio"},
		{string(ancoraggio.CollocazioneFuoriRichiesta), "fuori_richiesta"},
		{string(ancoraggio.CollocazioneNonDeterminabile), "non_determinabile"},
		{ancoraggio.LivelloProdotto, "prodotto"},
		{ancoraggio.LivelloComponente, "componente"},
		{ancoraggio.MotivoAncoraggioTargetIgnoti, "target_ignoti"},
		{ancoraggio.MotivoAncoraggioGrafoIncompleto, "ancoraggio.grafo_incompleto"},
		{ancoraggio.MotivoAncoraggioTargetSenzaStruttura, "ancoraggio.target_senza_struttura"},
		{ancoraggio.MotivoAncoraggioNessunaLettura, "nessuna_lettura_identita"},
		{ancoraggio.MotivoAncoraggioLivelliDiversi, "livelli_diversi"},
		{ancoraggio.MotivoAncoraggioIdentitaDiscordanti, "identita_discordanti"},
		{ancoraggio.MotivoAncoraggioRevisioniDiscordanti, "revisioni_discordanti"},
		{ancoraggio.MotivoAncoraggioCompletamentiMultipli, "completamenti_multipli"},
		{ancoraggio.MotivoAncoraggioCandidatiMultipli, "candidati_multipli"},
		{ancoraggio.MotivoCandidatoBaseDelTarget, "base_del_target"},
		{ancoraggio.MotivoCandidatoBaseDelNodo, "base_di_un_nodo_raggiungibile"},
		{ancoraggio.MotivoCandidatoRadiceScelta, "radice_scelta"},
		{ancoraggio.DimensioneNamespace, "namespace"},
		{ancoraggio.DimensioneBase, "base"},
		{ancoraggio.DimensioneMarcatore, "marcatore"},
		{ancoraggio.DimensioneRevisione, "revisione"},
		{ancoraggio.DimensioneQualificatori, "qualificatori"},
		{ancoraggio.DimensioneCodiceConfermato, "codice_confermato"},
		{ancoraggio.DimensioneCodiceManuale, "codice_manuale"},
		{ancoraggio.MotivoAbbinamentoPerBase, "abbinamento_per_base"},
		{ancoraggio.MotivoAbbinamentoComponentiStessaBase, "componenti_stessa_base"},
		{ancoraggio.MotivoAbbinamentoAccantoAllaRigaDecisa, "abbinamento_accanto_alla_riga_decisa"},
		{ancoraggio.MotivoNodoScartato, "nodo_scartato"},
		{ancoraggio.MotivoAbbinamentoDecisioneNonCompatibile, "decisione_non_compatibile"},
		{ancoraggio.MotivoAbbinamentoDecisioneNonVerificabile, "decisione_non_verificabile"},
		{ancoraggio.RifIdentita("acme", "7120100", "A", "2"), "identita:acme:7120100:A:2"},
		{ancoraggio.RifIdentita("acme", "7120100", "", ""), "identita:acme:7120100::"},
	} {
		if c.got != c.want {
			t.Errorf("valore %q, il contratto dice %q", c.got, c.want)
		}
	}
	for _, c := range []struct {
		tipo  any
		campi string
	}{
		{ancoraggio.EsitoAncoraggi{}, "VersioneServizio:versione_servizio HashIngresso:hash_ingresso HashTarget:hash_target File:file Strutture:strutture " +
			"Diagnostiche:diagnostiche Impronta:impronta"},
		{ancoraggio.AncoraggioFile{}, "AllegatoID:allegato_id Disponibilita:disponibilita Associazione:associazione Collocazione:collocazione Letture:letture " +
			"Candidati:candidati Motivi:motivi"},
		{ancoraggio.CandidatoAncoraggio{}, "Target:target Livello:livello Autorita:autorita Origine:origine RadiciDelFile:radici_del_file " +
			"RadiciRaggiungibili:radici_raggiungibili Padri:padri Percorsi:percorsi Archi:archi Posizioni:posizioni Letture:letture Dimensioni:dimensioni " +
			"Conflitti:conflitti IdentitaParziale:identita_parziale MotiviIdentita:motivi_identita Motivo:motivo"},
		{ancoraggio.PosizioneCandidato{}, "Target:target AllegatoID:allegato_id Radice:radice Nodo:nodo Stato:stato Motivo:motivo"},
		{ancoraggio.DimensioneCompatibilita{}, "Dimensione:dimensione Esito:esito Lettura:lettura Rif:rif"},
		{ancoraggio.AbbinamentoPerBase{}, "Componenti:componenti Motivo:motivo Decisione:decisione MotivoDecisione:motivo_decisione Compatibilita:compatibilita"},
		{ancoraggio.FileInterpretato{}, "AllegatoID: Documento: Interpretazione: Disponibilita: Disegno:"}, // riscritta per la fase 3 (T-B4-31)
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

// TestUnAltroMarcatoreEIlNodoSenzaMarcatore (T-B4-06 rivisto, T-B4-30; R86): il nodo 7120101 di un secondo STEP non
// scrive il marcatore e sta nel gruppo dell'unico marcatore scritto della sua base («A», il nodo del primo STEP). Un 2D
// «7120101B1» non è compatibile con «A», ma lo è, da una parte sola, con il nodo senza marcatore: è un candidato solo su
// quel nodo, mai fuori richiesta. Un 2D «7120101A1» resta compatibile con tutto il gruppo.
func TestUnAltroMarcatoreEIlNodoSenzaMarcatore(t *testing.T) {
	m := motoreCatena(t)
	a := scenaC(t, m, "telaio.stp")
	shaB := shaS("9")
	b := stepR(t, m, uidS(0x6b1), "supporto.stp", shaB, []nodoR{{"#1", "7120100A", "TELAIO", ""}, {"#2", "7120101A", "SUPPORTO", ""}}, []arcoS{{"#1", "#2", 1, nil}})
	for i := range b.Nodi {
		if b.Nodi[i].Chiave == "#2" {
			b.Nodi[i].Letture = append([]motorea.LetturaCodice(nil), b.Nodi[i].Letture...)
			for j := range b.Nodi[i].Letture {
				b.Nodi[i].Letture[j].Forma.Marcatore = nil // costruita a mano: una forma che non scrive il marcatore
			}
		}
	}
	tg := targetC(t, m, "identificativo:7120100A", ancoraggio.AutoritaConfermata, "7120100A", nil)
	ctx := ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{a, b}, CodiciProposti: codiciProposti(m, a, b)}
	conB := pdfF(t, m, uidS(0x6b2), "supporto-b.pdf", shaS("1"), []campoP{{"codice", "7120101B1"}})
	conA := pdfF(t, m, uidS(0x6b3), "supporto-a.pdf", shaS("2"), []campoP{{"codice", "7120101A1"}})
	e := ancoraA(t, []ancoraggio.FileInterpretato{conB, conA}, []ancoraggio.ProdottoRichiesto{tg}, ctx)
	x := ancoraggioDi(t, e, conB.AllegatoID)
	if x.Associazione != ancoraggio.AssociazioneCandidatoUnico || x.Collocazione != ancoraggio.CollocazioneFiglio || len(x.Candidati) != 1 {
		t.Fatalf("il 2D con un altro marcatore e il nodo senza marcatore: %+v", x)
	}
	c := x.Candidati[0]
	if c.Target != ancoraggio.RifNodo(shaB, "#2") || len(c.Posizioni) != 1 || c.Posizioni[0].Nodo != ancoraggio.RifNodo(shaB, "#2") ||
		dimensione(c, ancoraggio.DimensioneMarcatore, ancoraggio.RifNodo(shaB, "#2")) != motorea.CompatibilitaParziale {
		t.Errorf("il candidato solo sul nodo senza marcatore, con il marcatore compatibile da una parte sola: %+v", c)
	}
	y := ancoraggioDi(t, e, conA.AllegatoID)
	if len(y.Candidati) != 1 || len(y.Candidati[0].Posizioni) != 2 {
		t.Errorf("il 2D con il marcatore del gruppo sta su tutti e due i nodi: %+v", y.Candidati)
	}
}
