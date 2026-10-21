// L1 — l'adattatore del registro dopo la revisione della corsia motore (V1, ondata B, secondo passaggio): le diagnostiche
// che entrano nel registro solo se parlano del gruppo (EM §3.8, §3.8.1 «Perché le evidenze non entrano nel confronto»,
// §6.11; A-16), i file non attivi che non entrano in nessun punto del contenuto salvo la nota (EM §3.8 «proposta.file[]»,
// §9.9; A-14), le diagnostiche dell'adattatore (EM §6.1, «Errori dell'importazione»), le occorrenze e le «altre» di un
// arco (A-15) e l'emittente di un equivalente K4 preso dalla prova (A-06, RC-12). Come le altre scene: Calcola sulle
// fotografie sintetiche, RegoleDiLettura, ImportaGruppo, la rilettura stretta. Dove una prova vuole un caso che le
// grammatiche non danno (una relazione con una lettura che non c'è, due emittenti sullo stesso namespace), l'esito
// calcolato si cambia nella prova, ed è detto. Tutto è inventato: ACME, BETA, i codici 712xxxx e PNxxxxx, gli UUID.
package valutazione_test

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
)

// percorsiCon: i percorsi JSON del registro canonico in cui un testo compare (nei valori).
func percorsiCon(t *testing.T, canonico []byte, testo string) []string {
	t.Helper()
	var v any
	if err := json.Unmarshal(canonico, &v); err != nil {
		t.Fatal(err)
	}
	var out []string
	var visita func(p string, x any)
	visita = func(p string, x any) {
		switch y := x.(type) {
		case map[string]any:
			for k, w := range y {
				visita(p+"."+k, w)
			}
		case []any:
			for i, w := range y {
				visita(fmt.Sprintf("%s[%d]", p, i), w)
			}
		case string:
			if strings.Contains(y, testo) {
				out = append(out, p)
			}
		}
	}
	visita("", v)
	sort.Strings(out)
	return out
}

// soloNellaNota: l'allegato non compare in nessun punto del registro, salvo la nota importazione.evidenze_file_non_attivi
// della testata, che deve esserci e citarlo.
func soloNellaNota(t *testing.T, r importazione.Registro, a uuid.UUID) {
	t.Helper()
	nota := -1
	var dd []struct {
		Codice string   `json:"codice"`
		Rif    []string `json:"rif"`
	}
	_ = json.Unmarshal(r.Contenuto.Proposta.Diagnostiche, &dd)
	for i, d := range dd {
		if d.Codice == propostabom.CodiceImportazioneEvidenzeNonAttive {
			nota = i
			if !contieneTesto(d.Rif, "allegato:"+a.String()) {
				t.Errorf("la nota delle evidenze non attive non cita l'allegato %s: %+v", a, d)
			}
		}
	}
	if nota < 0 {
		t.Fatalf("manca la nota %s: %s", propostabom.CodiceImportazioneEvidenzeNonAttive, r.Contenuto.Proposta.Diagnostiche)
	}
	ammesso := fmt.Sprintf(".proposta.diagnostiche[%d].", nota)
	for _, p := range percorsiCon(t, r.Canonico, a.String()) {
		if !strings.HasPrefix(p, ammesso) {
			t.Errorf("l'allegato non attivo %s compare nel registro in %s", a, p)
		}
	}
	if contieneTesto(r.Contenuto.AllegatiCitati, a.String()) {
		t.Errorf("l'allegato non attivo %s è fra gli allegati citati", a)
	}
}

// diagnosticheDi: le diagnostiche di una voce o della testata, come elenco.
func diagnosticheDi(t *testing.T, raw json.RawMessage) []struct {
	Codice string   `json:"codice"`
	Rif    []string `json:"rif"`
} {
	t.Helper()
	var dd []struct {
		Codice string   `json:"codice"`
		Rif    []string `json:"rif"`
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &dd); err != nil {
			t.Fatalf("diagnostiche: %s", raw)
		}
	}
	return dd
}

// TestRevisioneLaCopiaDelloSTEPNonTocca (rilievo bloccante della revisione; EM §3.8.1, §6.11, §6.4.1 «due copie sono due
// occorrenze»; A-16): caricare una copia dello STEP (un altro allegato, lo stesso sha), attiva o in «Altri file», non
// cambia impronta_contenuto della BOM di lavoro: il rif dei nodi è uguale sulle due copie, e una diagnostica della
// struttura dell'altra copia non entra nelle voci. Nel registro della copia nessuna voce porta una diagnostica che cita
// l'altra occorrenza.
func TestRevisioneLaCopiaDelloSTEPNonTocca(t *testing.T) {
	prima := calcolaPerIlRegistro(t, scenaCollegamento(t), insiemeACME(t)).importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
	th := scenaCollegamento(t)
	conAllegato(&th, allegato(aStepGemello, 6, "copia-acme.stp", "stp", shaStepB), nil)
	c := calcolaPerIlRegistro(t, th, insiemeACME(t))

	t.Run("la copia caricata", func(t *testing.T) {
		dopo := c.importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
		x := propostabom.Confronta(prima, dopo)
		if prima.ImprontaContenuto != dopo.ImprontaContenuto || x.Esito != propostabom.EsitoUguale || !x.Differenze.Vuote() {
			t.Errorf("la copia caricata cambia la proposta della BOM: %s %+v", x.Esito, x.Differenze)
		}
	})
	t.Run("la copia in Altri file", func(t *testing.T) {
		r := c.importa(t, rifProdB, propostabom.GruppoBOM, propostabom.NonAttivi{aStepGemello: "altri_file"})
		x := propostabom.Confronta(prima, r.Contenuto)
		if prima.ImprontaContenuto != r.Contenuto.ImprontaContenuto || x.Esito != propostabom.EsitoUguale {
			t.Errorf("la copia in Altri file cambia la proposta della BOM: %s %+v", x.Esito, x.Differenze)
		}
		for _, p := range percorsiCon(t, r.Canonico, aStepGemello.String()) {
			if !strings.HasPrefix(p, ".proposta.diagnostiche[") {
				t.Errorf("la copia non attiva compare nel registro della BOM in %s", p)
			}
		}
	})
	t.Run("il registro della copia", func(t *testing.T) {
		pv := prodottoDi(t, c.et, rifProdB)
		var copia string
		for _, g := range propostabom.GruppiDelProdotto(pv, c.et.Ancoraggi.Strutture) {
			if g.AllegatoID != nil && *g.AllegatoID == aStepGemello {
				copia = g.Chiave
			}
		}
		if copia == "" {
			t.Fatal("la copia non ha un gruppo del prodotto")
		}
		k := c.importa(t, rifProdB, copia, nil).Contenuto
		for _, v := range k.Proposta.Nodi {
			if strings.Contains(string(v.Diagnostiche), "allegato:"+aStepB.String()) {
				t.Errorf("la voce %s della copia porta una diagnostica dell'altra occorrenza: %s", v.Voce, v.Diagnostiche)
			}
		}
		if strings.Contains(string(k.Proposta.Diagnostiche), "allegato:"+aStepB.String()) {
			t.Errorf("la testata della copia porta una diagnostica dell'altra occorrenza: %s", k.Proposta.Diagnostiche)
		}
	})
}

// TestRevisioneIFileNonAttiviNonEntrano (rilievi importanti della revisione; EM §3.8 «proposta.file[]: mai un'occorrenza
// non attiva», §9.9; A-14 per gli altri file): un file in «Altri file» o scartato non compare in nessun punto del registro
// (file, allegati citati, riferimenti, relazioni, proposte d'associazione, diagnostiche delle voci), salvo la nota che
// conta le evidenze lasciate fuori:
//   - il 2D dello sciolto scartato (riferimento documentale, file candidato, diagnostica della riconciliazione);
//   - la scansione assegnata in «Altri file» (riferimento documentale di un'associazione manuale);
//   - la prova K4 di un DXF in «Altri file»: senza prove l'equivalente non entra.
//
// In /dati le diagnostiche correnti dei nodi non lo citano.
func TestRevisioneIFileNonAttiviNonEntrano(t *testing.T) {
	c := calcolaPerIlRegistro(t, scenaCollegamento(t), insiemeACME(t))
	attivo := c.importa(t, rifProdB, propostabom.GruppoBOM, nil)
	if len(percorsiCon(t, attivo.Canonico, aDisegno.String())) == 0 || len(percorsiCon(t, attivo.Canonico, aScansione.String())) == 0 {
		t.Fatal("la scena non porta il 2D e la scansione nel registro: la prova non prova niente")
	}
	t.Run("il 2D scartato", func(t *testing.T) {
		na := propostabom.NonAttivi{aDisegno: "scartato"}
		r := c.importa(t, rifProdB, propostabom.GruppoBOM, na)
		soloNellaNota(t, r, aDisegno)
		e, et := c.e, c.et
		m := propostabom.Modello(propostabom.IngressoModello{Foto: &c.f, Esito: &e, Thread: &et, Regole: c.reg, Prodotto: rifProdB, NonAttivi: na})
		for _, g := range m.Gruppi {
			for _, n := range g.Nodi {
				if strings.Contains(string(n.Diagnostiche), aDisegno.String()) {
					t.Errorf("/dati: il nodo %s del gruppo %s cita il 2D scartato: %s", n.Rif, g.Chiave, n.Diagnostiche)
				}
			}
			for _, d := range diagnosticheDi(t, g.Diagnostiche) {
				if d.Codice != propostabom.CodiceImportazioneEvidenzeNonAttive && contieneTesto(d.Rif, "allegato:"+aDisegno.String()) {
					t.Errorf("/dati: la testata del gruppo %s cita il 2D scartato fuori dalla nota: %+v", g.Chiave, d)
				}
			}
		}
	})
	t.Run("la scansione in Altri file", func(t *testing.T) {
		soloNellaNota(t, c.importa(t, rifProdB, propostabom.GruppoBOM, propostabom.NonAttivi{aScansione: "altri_file"}), aScansione)
	})
	t.Run("la prova K4 in Altri file", func(t *testing.T) {
		th := scenaBOM(t, []nodoF{{"#1", "7120100A", ""}, {"#2", "7120200A", ""}, {"#3", "7120300A", ""}}, []arcoF{{"#1", "#2", 1}, {"#1", "#3", 1}})
		dxf := fattiDXF(t, shaDxfK4, [2]string{"riferimento_3d", "7120300A"}, [2]string{"codice", "PN93000"})
		conAllegato(&th, allegato(aDxfK4, 2, "disegno-k4.dxf", "dxf", shaDxfK4), &dxf)
		k := calcolaPerIlRegistro(t, th, insiemeK4(t))
		if !strings.Contains(string(voceDelRegistro(t, k.importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto, nodoB("#3")).Riferimenti), "equivalente_k4") {
			t.Fatal("con il DXF attivo #3 non ha l'equivalente: la prova non prova niente")
		}
		r := k.importa(t, rifProdB, propostabom.GruppoBOM, propostabom.NonAttivi{aDxfK4: "altri_file"})
		if strings.Contains(string(voceDelRegistro(t, r.Contenuto, nodoB("#3")).Riferimenti), "equivalente_k4") {
			t.Error("l'equivalente con la sola prova non attiva entra nel registro")
		}
		soloNellaNota(t, r, aDxfK4)
	})
}

// TestRevisioneLaDiagnosticaDiUnAltroProdotto (rilievo importante della revisione; EM §6.11, «un gesto qualunque sulla RFQ
// che non tocca il gruppo: no»): due prodotti nello stesso STEP, la riga di #4 propone «finito». La valutazione dà una
// diagnostica per prodotto sul nodo #4; nella voce #4 del registro del primo prodotto entra solo la sua, mai quella del
// secondo (che cita l'altro prodotto). La prova uccide anche il mutante che toglie le diagnostiche della valutazione.
func TestRevisioneLaDiagnosticaDiUnAltroProdotto(t *testing.T) {
	th := scenaDueTarget(t)
	rigaDi(t, &th, "#4").TipoProposto = testo("finito")
	c := calcolaPerIlRegistro(t, th, insiemeACME(t))
	nell := map[string]bool{}
	for _, d := range c.et.Diagnostiche {
		if d.Codice == codiceFinitoFuoriTarget && contieneTesto(d.Rif, nodoB("#4")) {
			nell[d.Rif[0]] = true
		}
	}
	if !nell[rifProdB] || !nell[rifSecondoTarget] {
		t.Fatalf("la scena non dà la diagnostica di #4 per i due prodotti: %v", nell)
	}
	v := voceDelRegistro(t, c.importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto, nodoB("#4"))
	dd := diagnosticheDi(t, v.Diagnostiche)
	if len(dd) != 1 || dd[0].Codice != codiceFinitoFuoriTarget || !contieneTesto(dd[0].Rif, rifProdB) || contieneTesto(dd[0].Rif, rifSecondoTarget) {
		t.Errorf("le diagnostiche della voce #4 del primo prodotto: %s", v.Diagnostiche)
	}
}

// TestRevisioneLeDiagnosticheDellAdattatore (rilievo importante della revisione; EM §6.1, «Errori dell'importazione»;
// EM §3.8 «proposta.relazioni[]»): i tre codici dell'adattatore che le scene non provavano.
//   - Senza la grammatica del cliente il thread si importa: importazione.senza_grammatica e niente grammatica_cliente.
//   - Una relazione che cita una lettura che l'interpretazione non ha non entra, con importazione.relazione_incompleta
//     (l'esito si cambia nella prova: il motore cita sempre letture sue).
//   - Lo stesso namespace con due emittenti: importazione.namespace_incoerente, con i due emittenti (l'esito si cambia
//     nella prova: le grammatiche non danno due emittenti sullo stesso namespace).
func TestRevisioneLeDiagnosticheDellAdattatore(t *testing.T) {
	t.Run("senza grammatica", func(t *testing.T) {
		th := scenaBOM(t, []nodoF{{"#1", "7120100A", ""}, {"#2", "7120200A", ""}}, []arcoF{{"#1", "#2", 1}})
		c := calcolaPerIlRegistro(t, th, insiemeDi(t))
		if c.et.Valutato {
			t.Fatal("senza grammatiche il thread risulta valutato")
		}
		k := c.importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
		if !strings.Contains(string(k.Proposta.Diagnostiche), `"codice":"`+propostabom.CodiceImportazioneSenzaGrammatica+`"`) {
			t.Errorf("manca importazione.senza_grammatica: %s", k.Proposta.Diagnostiche)
		}
		if k.Regole.GrammaticaCliente != nil {
			t.Errorf("grammatica_cliente senza grammatica: %+v", k.Regole.GrammaticaCliente)
		}
		for _, v := range k.Proposta.Nodi {
			if len(v.IdentitaProposta) != 0 || v.CodiceProposto != "" {
				t.Errorf("%s: un'identità senza grammatica: %s %q", v.Voce, v.IdentitaProposta, v.CodiceProposto)
			}
		}
	})
	t.Run("la relazione incompleta", func(t *testing.T) {
		_, hash := insiemeConLegami(t, false)
		r, _ := insiemeConLegami(t, false, contestoDeiLegami(hash, motorea.OccorrenzaContesto{AllegatoID: aStepB.String()}))
		c := calcolaPerIlRegistro(t, scenaLegame(t), r)
		intero := c.importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
		if len(intero.Proposta.Relazioni) != 2 {
			t.Fatalf("relazioni %d: attese le due della scena del legame", len(intero.Proposta.Relazioni))
		}
		var tolta string
		for i := range c.et.File {
			if f := &c.et.File[i]; f.AllegatoID == aStepB && len(f.Interpretazione.Relazioni) > 0 {
				rel := append([]motorea.RelazioneSemantica(nil), f.Interpretazione.Relazioni...)
				rel[0].LettureBersaglio = []string{"l:lettura-che-non-c-e"}
				tolta = rel[0].ID
				f.Interpretazione.Relazioni = rel
			}
		}
		k := c.importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
		if len(k.Proposta.Relazioni) != 1 || testoJSON(t, k.Proposta.Relazioni[0].IDMotore) == tolta {
			t.Errorf("la relazione con una lettura che non c'è entra nel registro: %+v", k.Proposta.Relazioni)
		}
		trovata := false
		for _, d := range diagnosticheDi(t, k.Proposta.Diagnostiche) {
			trovata = trovata || (d.Codice == propostabom.CodiceImportazioneRelazioneIncompleta && contieneTesto(d.Rif, tolta) &&
				contieneTesto(d.Rif, aStepB.String()+"/l:lettura-che-non-c-e"))
		}
		if !trovata {
			t.Errorf("manca importazione.relazione_incompleta con la relazione e la lettura: %s", k.Proposta.Diagnostiche)
		}
	})
	t.Run("due emittenti sullo stesso namespace", func(t *testing.T) {
		c := calcolaPerIlRegistro(t, scenaCollegamento(t), insiemeACME(t))
		n2 := nodoIn(t, bomDellEsito(t, c.et, rifProdB), nodoB("#2"))
		cambiata := false
		for i := range c.et.File {
			f := &c.et.File[i]
			if f.AllegatoID != aStepB {
				continue
			}
			ll := append([]motorea.LetturaCodice(nil), f.Interpretazione.Letture...)
			for j := range ll {
				if ll[j].ID == n2.Codice.Lettura {
					ll[j].Forma.Emittente, ll[j].Forma.HashEmittente = "beta", strings.Repeat("e", 64)
					cambiata = true
				}
			}
			f.Interpretazione.Letture = ll
		}
		if !cambiata {
			t.Fatal("la lettura scelta di #2 non c'è nell'interpretazione dello STEP")
		}
		k := c.importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
		trovata := false
		for _, d := range diagnosticheDi(t, k.Proposta.Diagnostiche) {
			trovata = trovata || (d.Codice == propostabom.CodiceImportazioneNamespaceIncoerente && contieneTesto(d.Rif, "partner:beta") &&
				contieneTesto(d.Rif, "cliente:"+clienteACME.String()))
		}
		if !trovata {
			t.Errorf("manca importazione.namespace_incoerente con i due emittenti: %s", k.Proposta.Diagnostiche)
		}
	})
}

// fattiStepConRighe: i fatti di uno STEP come fattiStepF, con le occorrenze (i NAUO) di un arco e le altre contate dal
// worker (evidenza.righe ed evidenza.altre, step_struttura.py).
func fattiStepConRighe(t *testing.T, sha string, nodi []nodoF, archi []arcoF, arco [2]string, righe []string, altre int) fotorfq.Fatti {
	t.Helper()
	n := []map[string]any{}
	for _, x := range nodi {
		n = append(n, map[string]any{"chiave": x.chiave, "id_grezzo": x.id, "nome_grezzo": "", "descrizione_grezza": "", "rev_grezza": x.rev, "evidenza": map[string]any{}})
	}
	r := []map[string]any{}
	for _, a := range archi {
		ev := map[string]any{}
		if a.padre == arco[0] && a.figlio == arco[1] {
			ev["righe"], ev["altre"] = righe, altre
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
	return fatti(t, sha, string(raw), testo(""))
}

// TestRevisioneA15LeOccorrenzeEAltre (A-15, la parte che mancava; EM §3.8 «proposta.archi[]»: occorrenze ≤ 20 e altre):
// un arco con 25 occorrenze elencate dal worker e 3 contate porta le prime 20 e altre = 3 + 5; un arco senza occorrenze
// non ha né l'una né l'altra.
func TestRevisioneA15LeOccorrenzeEAltre(t *testing.T) {
	nodi := []nodoF{{"#1", "7120100A", ""}, {"#2", "7120200A", ""}, {"#3", "7120300A", ""}}
	archi := []arcoF{{"#1", "#2", 28}, {"#1", "#3", 1}}
	var righe []string
	for i := 0; i < 25; i++ {
		righe = append(righe, fmt.Sprintf("#%d", 101+i))
	}
	th := scenaBOM(t, nodi, archi)
	th.Fatti[shaStepB] = fattiStepConRighe(t, shaStepB, nodi, archi, [2]string{"#1", "#2"}, righe, 3)
	k := calcolaPerIlRegistro(t, th, insiemeACME(t)).importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
	visti := 0
	for _, a := range k.Proposta.Archi {
		var occ []string
		var altre int
		_ = json.Unmarshal(a.Occorrenze, &occ)
		if len(a.Altre) > 0 {
			_ = json.Unmarshal(a.Altre, &altre)
		}
		switch a.Figlio {
		case nodoB("#2"):
			visti++
			if len(occ) != 20 || occ[0] != "#101" || occ[19] != "#120" || altre != 8 || a.Quantita == nil || *a.Quantita != 28 {
				t.Errorf("l'arco #1→#2: occorrenze %v, altre %d, quantità %v", occ, altre, a.Quantita)
			}
		case nodoB("#3"):
			visti++
			if len(occ) != 0 || len(a.Altre) != 0 {
				t.Errorf("l'arco #1→#3 senza occorrenze: %s %s", a.Occorrenze, a.Altre)
			}
		}
	}
	if visti != 2 {
		t.Fatalf("archi visti %d: %+v", visti, k.Proposta.Archi)
	}
}

// insiemeK4DelPartner: ACME con la famiglia della catena e la forma del riferimento 3D, e il partner BETA che legge il
// codice della parte nel cartiglio (PN e cinque cifre, namespace beta-pn), dichiarato da ACME con l'hash della sua
// grammatica; poi i contesti dati. Dà anche l'hash di BETA.
func insiemeK4DelPartner(t *testing.T, contesti ...motorea.ContestoLavorazione) (*motorea.InsiemeRegole, string) {
	t.Helper()
	lim := limitiACME()
	sel := []string{"cartiglio.codice"}
	beta := grammatica.Grammatica{
		VersioneSchema: grammatica.VersioneSchema,
		Emittente:      &grammatica.EmittenteGrammatica{ID: "beta", Nome: "BETA (inventato)"},
		Profilo:        grammatica.Profilo{Stato: grammatica.ProfiloCompleto},
		Famiglie: []grammatica.FamigliaCodice{{
			ID: "beta-pn", Namespace: "beta-pn", Ruoli: []grammatica.Ruolo{grammatica.RuoloComponente},
			Base: grammatica.Base{Segmenti: []grammatica.SegmentoBase{{Nome: "pn", Pattern: "PN[0-9]{5}", Identitario: true}},
				Maiuscole: grammatica.MaiuscoleEsatte, Normalizza: grammatica.NormalizzaNessuna,
				ConfinePrima: grammatica.ConfineAlnumASCII, ConfineDopo: grammatica.ConfineAlnumASCII},
			Forme: []grammatica.FormaCodice{{ID: "cartiglio", Selettori: sel, Stato: grammatica.StatoAttiva, Completa: true,
				Parti: []grammatica.Parte{{Tipo: grammatica.TipoParteBase, Min: 1, Max: 1}}}},
			Esempi: []grammatica.EsempioCodice{{ID: "e-pn", Origine: grammatica.OrigineSintetico, Selettore: "cartiglio.codice", Testo: "PN12345",
				Atteso: grammatica.AttesoEsempio{Letture: []grammatica.LetturaAttesa{{Forma: "cartiglio", Base: "PN12345"}}}}},
		}},
	}
	rawB, err := json.Marshal(beta)
	if err != nil {
		t.Fatal(err)
	}
	sb, err := grammatica.NuovoSnapshot(rawB, lim)
	if err != nil {
		t.Fatalf("BETA: %v", err)
	}
	f := famigliaCatena()
	f.Forme = append(f.Forme, formaCatena("cartiglio-3d", []string{"cartiglio.riferimento_3d"}, parteC(grammatica.TipoParteBase), parteC(grammatica.TipoParteMarcatore, "A", "B")))
	f.Esempi = append(f.Esempi, grammatica.EsempioCodice{ID: "e-3d", Origine: grammatica.OrigineSintetico, Selettore: "cartiglio.riferimento_3d", Testo: "7120100A",
		Atteso: grammatica.AttesoEsempio{Letture: []grammatica.LetturaAttesa{{Forma: "cartiglio-3d", Base: "7120100", Marcatore: "A"}}}})
	acme := grammatica.Grammatica{
		VersioneSchema: grammatica.VersioneSchema,
		Cliente:        grammatica.ClienteGrammatica{ID: clienteACME, RagioneSociale: "ACME S.p.A."},
		Profilo:        grammatica.Profilo{Stato: grammatica.ProfiloParziale},
		Famiglie:       []grammatica.FamigliaCodice{f},
		Partner:        []grammatica.DichiarazionePartner{{Emittente: "beta", HashSnapshot: sb.Hash, Selettori: sel}},
	}
	rawA, err := json.Marshal(acme)
	if err != nil {
		t.Fatal(err)
	}
	ind := grammatica.IndiceRegole{VersioneIndice: grammatica.VersioneIndice, Limiti: lim,
		Grammatiche: []grammatica.VoceIndice{{ClienteID: clienteACME, File: "acme.v1.json", Sha256: shaDi(rawA)}},
		Emittenti:   []grammatica.VoceEmittente{{ID: "beta", File: "beta.v1.json", Sha256: shaDi(rawB)}}}
	r, d, err := motorea.CompilaInsieme(ind, map[string][]byte{"acme.v1.json": rawA, "beta.v1.json": rawB})
	if err != nil || r.Motori[clienteACME] == nil || r.Emittenti["beta"] == nil {
		t.Fatalf("l'insieme K4 con il partner: %v\n%+v", err, d)
	}
	r.Contesti = contesti
	return &r, sb.Hash
}

// TestRevisioneA06LEmittenteDellaProva (A-06, la parte che mancava; EM §3.8 «equivalente.emittente è quello della lettura
// del codice della parte citata dalle prove ... mai l'emittente del nodo»; RC-12): il nodo #3 letto da ACME, il codice
// della parte del cartiglio del DXF letto dal partner BETA (il DXF nel perimetro del contesto). L'equivalente di #3 ha
// l'emittente di BETA, non quello del nodo; l'identità e il codice di partenza restano quelli dello STEP.
func TestRevisioneA06LEmittenteDellaProva(t *testing.T) {
	_, hash := insiemeK4DelPartner(t)
	r, _ := insiemeK4DelPartner(t, contestoACME(threadACME, hash, motorea.OccorrenzaContesto{AllegatoID: aDxfK4.String()}))
	th := scenaBOM(t, []nodoF{{"#1", "7120100A", ""}, {"#2", "7120200A", ""}, {"#3", "7120300A", ""}}, []arcoF{{"#1", "#2", 1}, {"#1", "#3", 1}})
	dxf := fattiDXF(t, shaDxfK4, [2]string{"riferimento_3d", "7120300A"}, [2]string{"codice", "PN93000"})
	conAllegato(&th, allegato(aDxfK4, 2, "disegno-k4.dxf", "dxf", shaDxfK4), &dxf)
	k := calcolaPerIlRegistro(t, th, r).importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
	v := voceDelRegistro(t, k, nodoB("#3"))
	if l := letturaDa(t, v.IdentitaProposta); l.Emittente != "cliente:"+clienteACME.String() || l.Letterale != "7120300A" || v.CodiceProposto != "7120300A" {
		t.Fatalf("l'identità di #3: %+v, codice %q", l, v.CodiceProposto)
	}
	var eq struct {
		Namespace string `json:"namespace"`
		Base      string `json:"base"`
		Emittente string `json:"emittente"`
	}
	trovato := false
	for _, x := range riferimentiDi(t, v) {
		if x.Tipo == "equivalente_k4" {
			trovato = true
			if err := json.Unmarshal(x.Equivalente, &eq); err != nil {
				t.Fatal(err)
			}
		}
	}
	if !trovato {
		t.Fatalf("#3 senza l'equivalente K4 del partner: %s", v.Riferimenti)
	}
	if eq.Namespace != "beta-pn" || eq.Base != "PN93000" || eq.Emittente != "partner:beta" {
		t.Errorf("l'equivalente: %+v (l'emittente è quello della lettura della prova, partner:beta)", eq)
	}
}
