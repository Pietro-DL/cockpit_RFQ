package bancoa

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/confronto"
	"promatec/cockpit/internal/core/estrazione"
	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
	valut "promatec/cockpit/internal/core/valutazione"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// L1 — ciò che la prima corsa vera di Q10 (08/10) chiede al banco (B6c):
//   - le 37 chiavi degli attesi che la corsa ha detto non tradotte, ognuna con il suo trattamento: verificata contro la
//     fotografia (gli ID e i campi delle righe decise, il thread delle voci e della C5, i conteggi), oppure dichiarata
//     (le chiavi descrittive fra le chiavi libere, l'esito con il target scritto che resta riservato); mai buttata;
//   - la sigla dell'utente nei campi deciso_da e confermato_da della C5, contro gli utenti della fotografia, e la
//     provenienza del documento (T-B6c-02);
//   - le decisioni dell'utente dell'08/10: D-R116 I (le righe della «Conferma l'albero» informative), O (la base dei
//     reali giudicata sul nome, che si legge per intero), F (con gli export il n.1 senza i finiti fuori perimetro),
//     D-R115 A (i clienti degli export fuori perimetro, con l'uscita 3, anche nella chiusura); D-R114 B è in
//     sezioni_rapporto_test.go (TestIlTestoDellaMisura), la tabella delle classi in risposte_test.go;
//   - nell_export_proposte, il nome neutro della riga dell'export della C5: la chiave di prima non ha alias;
//   - le correzioni decise dall'utente il 10/10: C-04 (D-2: la base dei reali si giudica sul nome solo se il nome è tutto
//     il file) e C-05 (deciso_da della sezione C5 contro gli utenti della fotografia, T-B6c-15).
//
// Ogni prova fissa una correzione: tolta la correzione, la prova cade (le mutazioni con -overlay sono nello scratchpad
// dell'agente, non nel repository). Le chiavi hanno i nomi del piano e della corsa; nessun valore degli attesi veri sta
// qui.
//
// I clienti di questi test sono inventati: vedi scena_banco_test.go.

// chiaviDellaPrimaCorsa: i percorsi delle 37 chiavi che la prima corsa vera ha detto non tradotte (32 da LeggiSezioni e 5
// nomi di conteggi dal n.5), senza valori.
var chiaviDellaPrimaCorsa = []string{
	"baseline_decisioni[].atteso.componente_id_proposta_vuoto_preservato",
	"baseline_decisioni[].atteso.decisioni_invariate",
	"baseline_decisioni[].atteso.revisione_confrontata_separatamente",
	"baseline_decisioni[].decisione_documento.allegato_id",
	"baseline_decisioni[].decisione_documento.estensione",
	"baseline_decisioni[].decisione_documento.messaggio_id",
	"baseline_decisioni[].decisione_documento.thread_id",
	"baseline_decisioni[].decisione_proposta.proposta_id",
	"baseline_decisioni[].thread_id",
	"decisioni_successive_da_rivedere.anomalie",
	"decisioni_successive_da_rivedere.cliente",
	"decisioni_successive_da_rivedere.decisione",
	"decisioni_successive_da_rivedere.deciso_da",
	"decisioni_successive_da_rivedere.file_confermati[].atteso",
	"decisioni_successive_da_rivedere.file_confermati[].decisione_documento.allegato_id",
	"decisioni_successive_da_rivedere.file_confermati[].decisione_documento.confermato_da",
	"decisioni_successive_da_rivedere.file_confermati[].decisione_documento.estensione",
	"decisioni_successive_da_rivedere.file_confermati[].decisione_documento.messaggio_id",
	"decisioni_successive_da_rivedere.file_confermati[].decisione_documento.thread_id",
	"decisioni_successive_da_rivedere.file_confermati[].decisione_proposta.proposta_id",
	"decisioni_successive_da_rivedere.file_confermati[].thread_id",
	"decisioni_successive_da_rivedere.fonte",
	"decisioni_successive_da_rivedere.intervallo",
	"decisioni_successive_da_rivedere.regole",
	"decisioni_successive_da_rivedere.stato",
	"decisioni_successive_da_rivedere.thread_id",
	"reali_archivi[].atteso.esito_con_target",
	"reali_archivi[].atteso.target_presente",
	"reali_archivi[].atteso.token_00_conservato",
	"scenario_inoltro.file[].codice",
	"scenario_inoltro.fonte_quantita",
	"scenario_inoltro.origine_atteso",
	"scenario_inoltro.conteggi.allegati",
	"scenario_inoltro.conteggi.candidati_figlio",
	"scenario_inoltro.conteggi.candidati_radice",
	"scenario_inoltro.conteggi.file_condivisi",
	"scenario_inoltro.conteggi.hash_distinti",
}

// formaDellaCorsa: gli attesi ACME con le 37 chiavi della prima corsa, nei posti in cui la corsa le ha trovate: dentro
// atteso (la baseline, i reali), nelle righe decise, nelle voci, nella sezione della C5 e nello scenario. I valori sono
// quelli della scena, scelti perché ogni verifica passi.
func formaDellaCorsa(t *testing.T) func(string) string {
	return func(a string) string {
		// lo scenario: la fonte della quantità della sezione, l'origine dell'atteso, i conteggi, il codice di un file
		a = sostituisci(t, a, "  prodotti_confermati_db: false\n",
			"  prodotti_confermati_db: false\n  fonte_quantita: cella\n  origine_atteso: inventata per la prova\n")
		a = sostituisci(t, a, "    prodotti: 1\n",
			"    prodotti: 1\n    candidati_radice: 1\n    candidati_figlio: 1\n    file_condivisi: 0\n    allegati: 3\n    hash_distinti: 3\n")
		a = sostituisci(t, a, "      nome_file: 9123456A_2.pdf\n      atteso: radice\n",
			"      nome_file: 9123456A_2.pdf\n      codice: 9123456A_2\n      atteso: radice\n")
		// la baseline: tre chiavi della voce dentro atteso, il thread della voce, gli ID delle righe decise
		a = sostituisci(t, a, "0111\"\n    atteso:\n      base: 9123456\n",
			"0111\"\n    thread_id: "+threadDecisioni.String()+"\n    atteso:\n      base: 9123456\n      decisioni_invariate: true\n"+
				"      revisione_confrontata_separatamente: true\n      componente_id_proposta_vuoto_preservato: false\n")
		a = sostituisci(t, a, "    componente_id_proposta_vuoto_preservato: false\nreali_archivi:", "reali_archivi:")
		a = sostituisci(t, a, "      deciso_il: \"2026-10-01T09:00:00.000Z\"\n",
			"      deciso_il: \"2026-10-01T09:00:00.000Z\"\n      proposta_id: "+uidACME(0x342).String()+"\n")
		a = sostituisci(t, a, "      sostituito_da: null\n",
			"      sostituito_da: null\n      allegato_id: "+allBaseline.String()+"\n      messaggio_id: "+msgDecisioni.String()+
				"\n      thread_id: "+threadDecisioni.String()+"\n      estensione: pdf\n")
		// i reali: target_presente ed esito_con_target dentro atteso, e il token «00» con il nome del piano
		a = sostituisci(t, a, "      revisione_dal_nome: null\n    target_presente: true\n",
			"      revisione_dal_nome: null\n      target_presente: true\n      esito_con_target: null\n      token_00_conservato: false\n")
		// la C5: le chiavi della sezione, il thread e l'atteso della voce, gli ID e i campi delle righe decise
		a = sostituisci(t, a, "decisioni_successive_da_rivedere:\n",
			"decisioni_successive_da_rivedere:\n  thread_id: "+threadDecisioni.String()+"\n  cliente: ACME\n  anomalie: [A-ACME-2]\n"+
				"  decisione: da rivedere, fuori dal gate\n  deciso_da: ACM\n  fonte: prova inventata\n  intervallo: 2026-10-01/2026-10-02\n"+
				"  regole: [nessuna]\n  stato: da_rivedere\n")
		a = sostituisci(t, a, "    - id: rivedere-acme-01\n      allegato_id: 00000000-0000-4000-8000-000000000112\n",
			"    - id: rivedere-acme-01\n      allegato_id: 00000000-0000-4000-8000-000000000112\n      thread_id: "+threadDecisioni.String()+
				"\n      atteso: da_rivedere\n")
		a = sostituisci(t, a, "        stato: confermata\n        componente_id: 00000000-0000-4000-8000-000000000032\n",
			"        stato: confermata\n        componente_id: 00000000-0000-4000-8000-000000000032\n        proposta_id: "+uidACME(0x343).String()+
				"\n        deciso_da: ACM\n")
		return sostituisci(t, a, "        documento_id: 00000000-0000-4000-8000-000000000222\n",
			"        documento_id: 00000000-0000-4000-8000-000000000222\n        allegato_id: "+allDaRivedere.String()+
				"\n        messaggio_id: "+msgDecisioni.String()+"\n        thread_id: "+threadDecisioni.String()+
				"\n        confermato_da: ACM\n        estensione: pdf\n")
	}
}

// percorsiDelleChiavi: i percorsi delle chiavi di un YAML a blocchi come quello della scena («[]» per un elemento di un
// elenco), per sapere quali chiavi una forma contiene. Basta per gli attesi ACME, scritti a blocchi; la libreria YAML sta
// solo in attesi.go.
func percorsiDelleChiavi(raw string) map[string]bool {
	type livello struct {
		rientro  int
		percorso string
	}
	var pila []livello
	padre := func(r int) string {
		for len(pila) > 0 && pila[len(pila)-1].rientro >= r {
			pila = pila[:len(pila)-1]
		}
		if len(pila) == 0 {
			return ""
		}
		return pila[len(pila)-1].percorso
	}
	out := map[string]bool{}
	for _, riga := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
		testo := strings.TrimLeft(riga, " ")
		if testo == "" || strings.HasPrefix(testo, "#") {
			continue
		}
		r := len(riga) - len(testo)
		if strings.HasPrefix(testo, "- ") {
			pila = append(pila, livello{r, padre(r) + "[]"})
			testo, r = testo[2:], r+2
		}
		i := strings.Index(testo, ":")
		if i <= 0 || strings.ContainsAny(testo[:i], " \"'[{") {
			continue
		}
		p := padre(r)
		if p != "" {
			p += "."
		}
		p += testo[:i]
		out[p] = true
		pila = append(pila, livello{r, p})
	}
	return out
}

// TestB6cLeChiaviDellaCorsaSiLeggono: la forma della corsa contiene tutte e 37 le chiavi; LeggiSezioni non ne lascia
// nessuna fra le non tradotte, il n.5 non conta conteggi sconosciuti, e ogni chiave arriva dove il suo trattamento la
// vuole.
func TestB6cLeChiaviDellaCorsaSiLeggono(t *testing.T) {
	raw := formaDellaCorsa(t)(leggiTestdata(t, "attesi_acme.yaml"))
	presenti := percorsiDelleChiavi(raw)
	if !presenti["decisioni_successive_da_rivedere.file_confermati[].nell_export_proposte.stato"] {
		t.Fatalf("il lettore dei percorsi della prova non legge la scena: %v", presenti)
	}
	for _, k := range chiaviDellaPrimaCorsa {
		if !presenti[k] {
			t.Errorf("la forma della corsa non contiene %s: la prova non la guarda", k)
		}
	}
	s := sezioniACME(t, formaDellaCorsa(t))
	if len(s.NonTradotte) != 0 || len(s.Errori) != 0 {
		t.Fatalf("non tradotte %v, errori %v", s.NonTradotte, s.Errori)
	}
	if c := controllaConteggi(s.Scenario); len(c) != 0 {
		t.Fatalf("il n.5 sui conteggi della corsa: %v", c)
	}
	b := s.Baseline[0]
	for _, k := range []string{"decisioni_invariate", "revisione_confrontata_separatamente", "componente_id_proposta_vuoto_preservato"} {
		if _, ok := b.invariante(k); !ok {
			t.Errorf("baseline: %s, scritta in atteso, non è fra gli invarianti: %+v", k, b.Invarianti)
		}
	}
	if _, ok := b.lettura("decisioni_invariate"); ok || b.ThreadID == nil || *b.ThreadID != threadDecisioni {
		t.Errorf("baseline: una chiave della voce fra le letture, o il thread non letto: %+v", b)
	}
	if c, ok := campo(b.Proposta, "proposta_id"); !ok || c.Valore != uidACME(0x342).String() {
		t.Errorf("proposta_id: %+v", b.Proposta)
	}
	for _, k := range []string{"allegato_id", "messaggio_id", "thread_id", "estensione"} {
		if _, ok := campo(b.Documento, k); !ok {
			t.Errorf("baseline, decisione_documento.%s non letta: %+v", k, b.Documento)
		}
	}
	re := s.Reali[0]
	if v, ok := re.invariante("target_presente"); !ok || !v || re.EsitoConTargetScritto {
		t.Errorf("reali: target_presente in atteso, esito_con_target null non scritto: %+v", re)
	}
	if _, ok := re.lettura("token_00_conservato"); !ok {
		t.Errorf("reali: token_00_conservato non è fra le letture: %+v", re.Letture)
	}
	c5 := s.DaRivedere[0]
	if c5.Atteso != "da_rivedere" || c5.ThreadID == nil {
		t.Errorf("C5: atteso e thread della voce: %+v", c5)
	}
	if c, ok := campo(c5.Proposta, "deciso_da"); !ok || c.Valore != "ACM" {
		t.Errorf("C5: la sigla di deciso_da: %+v", c5.Proposta)
	}
	for _, k := range []string{"allegato_id", "messaggio_id", "thread_id", "confermato_da", "estensione"} {
		if _, ok := campo(c5.Documento, k); !ok {
			t.Errorf("C5, decisione_documento.%s non letta: %+v", k, c5.Documento)
		}
	}
	if s.ThreadDaRivedere == nil || s.ThreadDaRivedere.Valore != threadDecisioni.String() || s.ClienteDaRivedere != nil {
		t.Errorf("C5: il thread della sezione, e il cliente scritto con il nome non è un UUID: %+v %+v", s.ThreadDaRivedere, s.ClienteDaRivedere)
	}
	if sc := s.Scenario; sc.FonteQuantita != "cella" || sc.Prodotti[0].FonteQuantita != "cella" || !strings.Contains(strings.Join(sc.File[0].Riportate, ";"), "codice = 9123456A_2") {
		t.Errorf("scenario: %+v", sc)
	}
	// Le chiavi descrittive: riportate, e fra le chiavi libere non verificate (con il codice dei file dello scenario).
	libere := " " + strings.Join(s.ChiaviLibere, " ") + " "
	for _, k := range []string{"scenario_inoltro.origine_atteso", "scenario_inoltro.file[].codice", "decisioni_successive_da_rivedere.cliente",
		"decisioni_successive_da_rivedere.decisione", "decisioni_successive_da_rivedere.fonte",
		"decisioni_successive_da_rivedere.intervallo", "decisioni_successive_da_rivedere.regole", "decisioni_successive_da_rivedere.stato"} {
		if !strings.Contains(libere, " "+k+" ") {
			t.Errorf("%s non è fra le chiavi libere: %v", k, s.ChiaviLibere)
		}
	}
	// deciso_da della sezione non è più descrittiva: va al n.2, contro gli utenti della fotografia (C-05, T-B6c-15).
	if strings.Contains(libere, " decisioni_successive_da_rivedere.deciso_da ") || s.DecisoDaRivedere == nil ||
		s.DecisoDaRivedere.Valore != "ACM" || s.DecisoDaRivedere.Percorso != "decisioni_successive_da_rivedere.deciso_da" {
		t.Errorf("deciso_da della C5: %+v, chiavi libere %v", s.DecisoDaRivedere, s.ChiaviLibere)
	}
	riportate := map[string]bool{}
	for _, r := range s.Riportate {
		riportate[r.Percorso] = true
	}
	for _, p := range []string{"decisioni_successive_da_rivedere.anomalie[0]", "decisioni_successive_da_rivedere.regole[0]",
		"decisioni_successive_da_rivedere.cliente", "scenario_inoltro.origine_atteso"} {
		if !riportate[p] {
			t.Errorf("%s non è fra le righe riportate: %+v", p, s.Riportate)
		}
	}
	if strings.Contains(libere, "anomalie") {
		t.Errorf("le anomalie della C5 si riportano (6.4.9), non sono chiavi libere: %v", s.ChiaviLibere)
	}
	// Una chiave della sezione C5 scritta vuota non si butta: il thread va al n.2, il cliente fra le descrittive.
	vuote := sezioniACME(t, func(a string) string {
		a = sostituisci(t, formaDellaCorsa(t)(a), "  thread_id: "+threadDecisioni.String()+"\n  cliente: ACME\n", "  thread_id: null\n  cliente: null\n")
		return a
	})
	if vuote.ThreadDaRivedere == nil || vuote.ThreadDaRivedere.Valore != "" || !strings.Contains(" "+strings.Join(vuote.ChiaviLibere, " ")+" ", " decisioni_successive_da_rivedere.cliente ") {
		t.Errorf("le chiavi vuote della sezione C5: thread %+v, chiavi libere %v", vuote.ThreadDaRivedere, vuote.ChiaviLibere)
	}
}

// TestB6cLaChiaveDellExportSenzaAlias: la riga dell'export della C5 si legge con il nome neutro nell_export_proposte;
// la chiave di prima, da qui in poi, è una chiave non tradotta come le altre. Il nome di prima portava un frammento del
// nome di un file dell'export, e qui non si scrive: la prova guarda che un altro nome della stessa famiglia non si
// legga, e che il codice del pacchetto non ne conosca nessun altro (nessun alias).
func TestB6cLaChiaveDellExportSenzaAlias(t *testing.T) {
	if s := sezioniACME(t, nil); len(s.DaRivedere[0].Export) != 2 {
		t.Fatalf("nell_export_proposte non letta: %+v", s.DaRivedere[0])
	}
	s := sezioniACME(t, func(a string) string {
		return sostituisci(t, a, "      nell_export_proposte:\n", "      nell_export_acme:\n")
	})
	if strings.Join(s.NonTradotte, "|") != "decisioni_successive_da_rivedere.file_confermati[].nell_export_acme" || len(s.DaRivedere[0].Export) != 0 {
		t.Errorf("un altro nome: non tradotte %v, riga dell'export %+v", s.NonTradotte, s.DaRivedere[0].Export)
	}
	// Le chiavi che una voce della C5 accetta: della riga dell'export solo il nome neutro (un alias passerebbe da qui,
	// perché LeggiSezioni legge solo le chiavi note della sezione).
	var export []string
	for _, k := range chiaviDaRivedere {
		if strings.HasPrefix(k, "nell_export_") {
			export = append(export, k)
		}
	}
	if strings.Join(export, " ") != "nell_export_proposte" {
		t.Errorf("le chiavi della riga dell'export nella C5: %v (nessun alias)", export)
	}
	nomi := regexp.MustCompile(`"nell_export_[a-z0-9_]*`)
	file, err := filepath.Glob("*.go")
	if err != nil || len(file) == 0 {
		t.Fatalf("i sorgenti del pacchetto: %v", err)
	}
	for _, f := range file {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range nomi.FindAllString(string(b), -1) {
			if m != `"nell_export_proposte` {
				t.Errorf("%s: un nome della riga dell'export diverso da quello neutro: %s (nessun alias)", f, m)
			}
		}
	}
}

// TestB6cLeChiaviDellaCorsaNellaScena: la corsa sugli export con le chiavi della prima corsa non ha differenze (il n.5
// non ne conta più); poi ogni chiave, cambiata, dà una differenza nel controllo che la verifica, o un errore di
// traduzione dove la voce si contraddice.
func TestB6cLeChiaviDellaCorsaNellaScena(t *testing.T) {
	forma := formaDellaCorsa(t)
	r := corsaBanco(t, attesiMutati(forma), true, false)
	if r.Differenze != 0 {
		t.Fatalf("con le chiavi della corsa la scena non ha differenze: %s\n%s", r.PrimaRiga(), r.Testo())
	}
	if c, _ := controlloBanco(r, ControlloAttesiInteri); c.Stato != ControlloEseguito || c.Differenze != 0 {
		t.Errorf("n.5: %+v", c)
	}
	nomi := map[string]bool{}
	for _, c := range r.Gate.ConteggiScenario {
		nomi[c.Nome] = true
		if (c.Nome == "candidati_radice" || c.Nome == "candidati_figlio") && c.Dichiarato != 1 {
			t.Errorf("il conteggio %s nel gate: %+v", c.Nome, c)
		}
	}
	if !nomi["candidati_radice"] || !nomi["candidati_figlio"] || !nomi["file_condivisi"] || nomi["allegati"] || nomi["hash_distinti"] {
		t.Errorf("i conteggi del gate: %+v (allegati e hash_distinti non sono esiti del motore)", r.Gate.ConteggiScenario)
	}
	// target_presente in atteso arriva alle letture; token_00_conservato si giudica con la regola di token_conservato
	// (false è un'asserzione negativa sull'ambito dedotto, fra le parti non verificate).
	d := dettaglio(r, ControlloLetture)
	if !conPrefisso(d.NonVerificate, "reali_archivi[0].target_presente: i componenti non sono nella fotografia") ||
		!conPrefisso(d.NonVerificate, "reali_archivi[0].atteso.token_00_conservato: ambito dedotto, asserzione negativa: sull'unità nome_file") {
		t.Errorf("le letture dei reali: %+v", d)
	}

	casi := map[string]struct {
		vecchio, nuovo, controllo, parte string
	}{
		"il thread della voce": {"0111\"\n    thread_id: " + threadDecisioni.String(), "0111\"\n    thread_id: " + threadScenario.String(),
			ControlloRisoluzione, "baseline_decisioni[0].thread_id: non è il thread dell'allegato"},
		"proposta_id": {"      proposta_id: " + uidACME(0x342).String(), "      proposta_id: " + uidACME(0x344).String(),
			ControlloBaselineRighe, "baseline_decisioni[0]: proposta.proposta_id: diverso dalla fotografia"},
		"allegato_id del documento": {"      sostituito_da: null\n      allegato_id: " + allBaseline.String(), "      sostituito_da: null\n      allegato_id: " + allDaRivedere.String(),
			ControlloBaselineRighe, "baseline_decisioni[0]: documento.allegato_id: non è fra gli allegati della provenienza del documento"},
		"messaggio_id del documento": {"      messaggio_id: " + msgDecisioni.String() + "\n      thread_id: " + threadDecisioni.String() + "\n      estensione: pdf\nreali",
			"      messaggio_id: " + msgScenario.String() + "\n      thread_id: " + threadDecisioni.String() + "\n      estensione: pdf\nreali",
			ControlloBaselineRighe, "baseline_decisioni[0]: documento.messaggio_id: non è il messaggio dell'allegato della provenienza"},
		"thread_id del documento": {"      thread_id: " + threadDecisioni.String() + "\n      estensione: pdf\nreali",
			"      thread_id: " + threadScenario.String() + "\n      estensione: pdf\nreali",
			ControlloBaselineRighe, "baseline_decisioni[0]: documento.thread_id: diverso dalla fotografia"},
		"estensione del documento": {"      estensione: pdf\nreali_archivi:", "      estensione: dxf\nreali_archivi:",
			ControlloBaselineRighe, "baseline_decisioni[0]: documento.estensione: diverso dalla fotografia"},
		"decisioni_invariate falso": {"      decisioni_invariate: true\n", "      decisioni_invariate: false\n",
			ControlloTraduzione, "baseline_decisioni[0]: atteso.decisioni_invariate falso"},
		"revisione_confrontata_separatamente falso": {"      revisione_confrontata_separatamente: true\n", "      revisione_confrontata_separatamente: false\n",
			ControlloLetture, "baseline_decisioni[0].atteso.revisione_confrontata_separatamente: falso"},
		"componente_id_proposta_vuoto_preservato in atteso": {"      componente_id_proposta_vuoto_preservato: false\n    decisione_proposta:",
			"      componente_id_proposta_vuoto_preservato: true\n    decisione_proposta:",
			ControlloBaselineRighe, "baseline_decisioni[0]: componente_id_proposta_vuoto_preservato non coincide"},
		"l'atteso della C5": {"      atteso: da_rivedere\n", "      atteso: corretto\n",
			ControlloTraduzione, "decisioni_successive_da_rivedere.file_confermati[0]: atteso \"corretto\""},
		"il thread della voce della C5": {"112\n      thread_id: " + threadDecisioni.String(), "112\n      thread_id: " + threadScenario.String(),
			ControlloRisoluzione, "decisioni_successive_da_rivedere.file_confermati[0].thread_id: non è il thread dell'allegato"},
		"il thread della C5": {"decisioni_successive_da_rivedere:\n  thread_id: " + threadDecisioni.String(),
			"decisioni_successive_da_rivedere:\n  thread_id: " + uidACME(0x79).String(),
			ControlloRisoluzione, "decisioni_successive_da_rivedere.thread_id: non si risolve nella fotografia"},
		"il cliente della C5 con l'UUID di un altro": {"  cliente: ACME\n", "  cliente: " + uidACME(0xac09).String() + "\n",
			ControlloRisoluzione, "decisioni_successive_da_rivedere.cliente: non è il cliente del thread della sezione"},
		"fonte_quantita diversa": {"  fonte_quantita: cella\n", "  fonte_quantita: riga\n",
			ControlloTraduzione, "scenario_inoltro.prodotti_attesi[0].fonte_quantita: diversa da fonte_quantita dello scenario"},
		"candidati_radice": {"    candidati_radice: 1\n", "    candidati_radice: 2\n",
			ControlloAttesiInteri, "scenario_inoltro.conteggi.candidati_radice dichiara 2, le voci sono 1"},
		"candidati_figlio": {"    candidati_figlio: 1\n", "    candidati_figlio: 0\n",
			ControlloAttesiInteri, "scenario_inoltro.conteggi.candidati_figlio dichiara 0, le voci sono 1"},
		"file_condivisi": {"    file_condivisi: 0\n", "    file_condivisi: 1\n",
			ControlloAttesiInteri, "scenario_inoltro.conteggi.file_condivisi dichiara 1, le voci sono 0"},
		"allegati": {"    allegati: 3\n", "    allegati: 4\n",
			ControlloAttesiInteri, "scenario_inoltro.conteggi.allegati dichiara 4, le voci sono 3"},
		"hash_distinti": {"    hash_distinti: 3\n", "    hash_distinti: 2\n",
			ControlloRisoluzione, "scenario_inoltro.conteggi.hash_distinti dichiara 2, gli allegati delle voci dello scenario nella fotografia hanno 3 sha256 distinti"},
	}
	for nome, c := range casi {
		t.Run(nome, func(t *testing.T) {
			r := corsaBanco(t, attesiMutati(func(a string) string { return sostituisci(t, forma(a), c.vecchio, c.nuovo) }), true, false)
			if x, _ := controlloBanco(r, c.controllo); x.Differenze == 0 || !conPrefisso(dettaglio(r, c.controllo).Differenze, c.parte) {
				t.Fatalf("%s: %+v, differenze %q", c.controllo, x, dettaglio(r, c.controllo).Differenze)
			}
			if r.Esito.CodiceUscita() != UscitaConDifferenze {
				t.Errorf("una differenza dà 1: %s", r.PrimaRiga())
			}
		})
	}

	// Il cliente della C5 con l'UUID del cliente del thread della sezione passa; l'esito con il target scritto resta
	// riservato, e si riporta accanto all'esito del file.
	r = corsaBanco(t, attesiMutati(func(a string) string {
		a = sostituisci(t, forma(a), "  cliente: ACME\n", "  cliente: "+clienteACME.String()+"\n")
		return sostituisci(t, a, "      esito_con_target: null\n", "      esito_con_target: non_coperto\n")
	}), true, false)
	if c, _ := controlloBanco(r, ControlloRisoluzione); c.Differenze != 0 {
		t.Errorf("il cliente giusto della C5: %+v, %q", c, dettaglio(r, ControlloRisoluzione).Differenze)
	}
	riportata := false
	for _, th := range r.Thread {
		for _, f := range th.File {
			for _, e := range f.Esiti {
				if e.Sezione == confronto.SezioneReali && e.Esito == confronto.EsitoRiservato && strings.Contains(strings.Join(e.Riportate, ";"), "esito_con_target = non_coperto") {
					riportata = true
				}
			}
		}
	}
	if !riportata || strings.Join(r.Gate.Riservati, " ") != "reali_archivi[0]" {
		t.Errorf("l'esito con il target scritto: riservato e riportato (%t), riservati %v", riportata, r.Gate.Riservati)
	}
}

// TestB6cIConteggiDellaCorsaNelGate (T-B6c-09; 6.4.9, la voce «conteggi» del gate: radice, figlio «con due radici»,
// fuori richiesta, prodotti): i conteggi della corsa che sono esiti del motore (candidati_radice, candidati_figlio,
// file_condivisi) si contano sugli esiti corretti; allegati e hash_distinti no (li verificano il n.5 e il n.2). Un figlio
// non corretto, o con una radice sola, non conta.
func TestB6cIConteggiDellaCorsaNelGate(t *testing.T) {
	in := ingressoGateACME()
	in.Scenario = &ScenarioAtteso{Conteggi: []ConteggioAtteso{{Nome: "allegati", Valore: 3}, {Nome: "candidati_figlio", Valore: 1},
		{Nome: "candidati_radice", Valore: 1}, {Nome: "file_condivisi", Valore: 1}, {Nome: "hash_distinti", Valore: 3}}}
	in.Voci[1].Radici = 2
	g := calcolaGate(in)
	var nomi []string
	for _, c := range g.ConteggiScenario {
		nomi = append(nomi, c.Nome)
		if c.Dichiarato != c.Ottenuto || c.InPiu != 0 {
			t.Errorf("il conteggio %s: %+v", c.Nome, c)
		}
	}
	if strings.Join(nomi, " ") != "candidati_figlio candidati_radice file_condivisi" || voce(g, VoceGateConteggiScenario).Stato != VoceSuperata {
		t.Fatalf("i conteggi della corsa nel gate: %v, %+v", nomi, voce(g, VoceGateConteggiScenario))
	}
	for nome, muta := range map[string]func(*ingressoGate){
		"un figlio non corretto":   func(in *ingressoGate) { in.Voci[1].Esito = confronto.EsitoErrato },
		"un figlio con una radice": func(in *ingressoGate) { in.Voci[1].Radici = 1 },
		"una radice non corretta":  func(in *ingressoGate) { in.Voci[0].Esito = confronto.EsitoAmbiguo },
	} {
		x := in
		x.Voci = append([]esitoVoce(nil), in.Voci...)
		muta(&x)
		if v := voce(calcolaGate(x), VoceGateConteggiScenario); v.Stato != VoceNonSuperata {
			t.Errorf("%s: %+v", nome, v)
		}
	}
}

// TestB6cDecisioniInvariateSenzaRighe: decisioni_invariate vero lo verificano le righe del controllo (1); una voce che
// non ne ha lo lascia non verificato, mai passato.
func TestB6cDecisioniInvariateSenzaRighe(t *testing.T) {
	s, ix := fotoEIndice(t)
	f := fotoDellaScena(t, s)
	ins := insiemeACME(t)
	sez := sezioniACME(t, formaDellaCorsa(t))
	sez.Baseline[0].Proposta, sez.Baseline[0].Documento = nil, nil
	eb := controllaBaseline(traduci(sez, f, ix, &ins), f, ix, nil, nil, contestoCorsa{export: true})
	if len(eb) != 1 || !conPrefisso(eb[0].NonVerificate, "atteso.decisioni_invariate: la voce non ha righe decise da confrontare") {
		t.Errorf("decisioni_invariate senza righe: %+v", eb)
	}
}

// TestB6cLaProvenienzaDelDocumento (T-B6c-02): la fotografia porta del documento gli allegati della provenienza, non le
// righe intere; il messaggio della provenienza si verifica come il messaggio dell'allegato della stessa riga. Ciò che la
// fotografia non può dire non si decide: è una parte non verificata, mai un passato né una differenza inventata.
func TestB6cLaProvenienzaDelDocumento(t *testing.T) {
	s, ix := fotoEIndice(t)
	f := fotoDellaScena(t, s)
	var doc fotorfq.DocumentoConfermato
	for _, th := range f.Thread {
		for _, d := range th.Documenti {
			if d.ID == docDaRivedere {
				doc = d
			}
		}
	}
	// Negli export l'allegato non esportato della provenienza è tolto (fuori dal thread): la sezione è parziale.
	if len(doc.Allegati) != 1 || doc.Allegati[0] != allDaRivedere {
		t.Fatalf("il documento della C5 nella scena: %+v", doc)
	}
	export := fontiDi(f, ix, true)
	completa := export
	completa.export, completa.provenienzeComplete = false, true
	if export.provenienzeComplete {
		t.Fatal("le provenienze degli export sono parziali")
	}
	al := func(v string) CampoAtteso { return CampoAtteso{Nome: "allegato_id", Valore: v} }
	ms := func(v string) CampoAtteso { return CampoAtteso{Nome: "messaggio_id", Valore: v} }
	nullo := func(nome string) CampoAtteso { return CampoAtteso{Nome: nome, Nullo: true} }
	for nome, c := range map[string]struct {
		x               CampoAtteso
		fr              fontiRighe
		differenza, non string
	}{
		"l'allegato della provenienza":                      {al(allDaRivedere.String()), export, "", ""},
		"un allegato del thread che il documento non porta": {al(allBaseline.String()), export, "non è fra gli allegati della provenienza del documento", ""},
		"un allegato tolto dagli export":                    {al(allNonEsportato.String()), export, "", "la sezione delle provenienze è parziale"},
		"lo stesso allegato con le provenienze complete":    {al(allNonEsportato.String()), completa, "non è fra gli allegati della provenienza del documento", ""},
		"null":                     {nullo("allegato_id"), completa, "", "null: una provenienza senza allegato non si distingue"},
		"un UUID che non si legge": {al("non-un-uuid"), export, "l'UUID non si legge", ""},
	} {
		d, n := allegatoDellaProvenienza(c.x, &doc, c.fr)
		if (c.differenza == "") != (d == "") || !strings.Contains(d, c.differenza) || (c.non == "") != (n == "") || !strings.Contains(n, c.non) {
			t.Errorf("%s: differenza %q, non verificata %q", nome, d, n)
		}
	}
	riga := func(allegato string) []CampoAtteso {
		if allegato == "" {
			return nil
		}
		return []CampoAtteso{al(allegato)}
	}
	for nome, c := range map[string]struct {
		x               CampoAtteso
		allegato        string
		differenza, non string
	}{
		"il messaggio dell'allegato":               {ms(msgDecisioni.String()), allDaRivedere.String(), "", ""},
		"un altro messaggio":                       {ms(msgScenario.String()), allDaRivedere.String(), "non è il messaggio dell'allegato della provenienza", ""},
		"senza l'allegato nella riga":              {ms(msgDecisioni.String()), "", "", "la riga non dice l'allegato della provenienza"},
		"un allegato che la fotografia non ha":     {ms(msgNonEsportato.String()), allNonEsportato.String(), "", "l'allegato della provenienza non è nella fotografia"},
		"null":                                     {nullo("messaggio_id"), allDaRivedere.String(), "", "null: la fotografia non porta il messaggio della provenienza"},
		"maiuscole diverse dello stesso messaggio": {ms(strings.ToUpper(msgDecisioni.String())), allDaRivedere.String(), "", ""},
	} {
		d, n := messaggioDellaProvenienza(c.x, riga(c.allegato), export)
		if (c.differenza == "") != (d == "") || !strings.Contains(d, c.differenza) || (c.non == "") != (n == "") || !strings.Contains(n, c.non) {
			t.Errorf("%s: differenza %q, non verificata %q", nome, d, n)
		}
	}
	// Nella corsa: un null nella riga del documento della baseline è una parte non verificata, mai una differenza.
	ins := insiemeACME(t)
	for nome, k := range map[string][2]string{
		"messaggio_id": {"null\n      allegato_id: " + allBaseline.String() + "\n      messaggio_id: " + msgDecisioni.String() + "\n",
			"null\n      allegato_id: " + allBaseline.String() + "\n      messaggio_id: null\n"},
		"allegato_id": {"null\n      allegato_id: " + allBaseline.String() + "\n", "null\n      allegato_id: null\n"},
	} {
		sez := sezioniACME(t, func(a string) string { return sostituisci(t, formaDellaCorsa(t)(a), k[0], k[1]) })
		eb := controllaBaseline(traduci(sez, f, ix, &ins), f, ix, nil, nil, contestoCorsa{export: true})
		if len(eb) != 1 || conPrefisso(eb[0].Differenze, "documento.") || !conPrefisso(eb[0].NonVerificate, "documento."+nome+" (null:") {
			t.Errorf("%s null: %+v", nome, eb)
		}
	}
}

// fotoConUtenti: la fotografia della scena come la darebbe la copia: gli utenti con le sigle, e chi ha deciso la
// proposta e confermato il documento del file della C5.
func fotoConUtenti(t *testing.T, s scenaBanco) fotorfq.Fotografia {
	t.Helper()
	f := fotoDellaScena(t, s)
	f.Utenti = []fotorfq.Utente{{ID: utenteACME, Sigla: "ACM"}}
	for i := range f.Thread {
		for j := range f.Thread[i].Proposte {
			if f.Thread[i].Proposte[j].AllegatoID == allDaRivedere {
				u := utenteACME
				f.Thread[i].Proposte[j].DecisoDa = &u
			}
		}
		for j := range f.Thread[i].Documenti {
			if f.Thread[i].Documenti[j].ID == docDaRivedere {
				f.Thread[i].Documenti[j].ConfermatoDa = utenteACME
			}
		}
	}
	return f
}

// TestB6cLaSiglaNeiCampiDa: con -dsn la C5 scrive chi ha deciso e chi ha confermato con la sigla; la fotografia ha
// l'UUID. La sigla si confronta con quella dell'utente della fotografia con quell'UUID: uguale passa (prima era
// «diverso dalla fotografia»), diversa è una differenza; un UUID atteso si confronta come prima; con -dsn un utente che
// la fotografia non ha è una differenza, anche se la fotografia non ne ha nessuno (la copia li ha tutti: non è un limite
// della corsa). Con gli export deciso_da resta non confrontabile, come prima, e una sigla di confermato_da non si
// confronta (gli export non hanno gli utenti). La riga del documento della C5 con -dsn: la provenienza, il thread,
// l'estensione.
func TestB6cLaSiglaNeiCampiDa(t *testing.T) {
	s, ix := fotoEIndice(t)
	f := fotoConUtenti(t, s)
	ins := insiemeACME(t)
	badge := map[uuid.UUID]confronto.Badge{allDaRivedere: confronto.BadgeUguale}
	c5 := func(muta func(string) string, f fotorfq.Fotografia, cc contestoCorsa) esitoControllo {
		sez := sezioniACME(t, func(a string) string { return muta(formaDellaCorsa(t)(a)) })
		return controlloInvariantiC5(&sez, traduci(sez, f, ix, &ins), f, ix, cc, badge)
	}
	nessuna := func(a string) string { return a }
	if e := c5(nessuna, f, contestoCorsa{}); len(e.differenze) != 0 || len(e.nonFatti) != 0 {
		t.Fatalf("le sigle giuste e la riga del documento: differenze %q, non fatti %q", e.differenze, e.nonFatti)
	}
	for nome, c := range map[string]struct {
		vecchio, nuovo, parte string
	}{
		"deciso_da con un'altra sigla":     {"        deciso_da: ACM\n", "        deciso_da: XYZ\n", "proposta.deciso_da: la sigla non è quella dell'utente della fotografia"},
		"confermato_da con un'altra sigla": {"        confermato_da: ACM\n", "        confermato_da: XYZ\n", "documento.confermato_da: la sigla non è quella dell'utente della fotografia"},
		"la sigla in minuscolo":            {"        deciso_da: ACM\n", "        deciso_da: acm\n", "proposta.deciso_da: la sigla non è quella dell'utente della fotografia"},
		"deciso_da con un altro UUID":      {"        deciso_da: ACM\n", "        deciso_da: " + uidACME(0x52).String() + "\n", "proposta.deciso_da: diverso dalla fotografia"},
		"il messaggio della provenienza":   {"        messaggio_id: " + msgDecisioni.String() + "\n", "        messaggio_id: " + msgScenario.String() + "\n", "documento.messaggio_id: non è il messaggio dell'allegato della provenienza nella fotografia"},
		"il thread del documento":          {"        thread_id: " + threadDecisioni.String() + "\n        confermato_da", "        thread_id: " + threadScenario.String() + "\n        confermato_da", "documento.thread_id: diverso dalla fotografia"},
		"l'estensione del documento":       {"        estensione: pdf\n", "        estensione: dxf\n", "documento.estensione: diverso dalla fotografia"},
	} {
		e := c5(func(a string) string { return sostituisci(t, a, c.vecchio, c.nuovo) }, f, contestoCorsa{})
		if len(e.differenze) != 1 || !strings.HasSuffix(e.differenze[0], c.parte) {
			t.Errorf("%s: differenze %q", nome, e.differenze)
		}
	}
	if e := c5(func(a string) string {
		return sostituisci(t, a, "        deciso_da: ACM\n", "        deciso_da: "+utenteACME.String()+"\n")
	}, f, contestoCorsa{}); len(e.differenze) != 0 {
		t.Errorf("l'UUID giusto si confronta come prima: %q", e.differenze)
	}
	altro := f
	altro.Utenti = []fotorfq.Utente{{ID: uidACME(0x52), Sigla: "ACM"}}
	senza := f
	senza.Utenti = nil
	for nome, g := range map[string]fotorfq.Fotografia{"l'UUID della fotografia senza utente": altro, "una fotografia senza utenti": senza} {
		if e := c5(nessuna, g, contestoCorsa{}); len(e.differenze) != 2 || len(e.nonFatti) != 0 ||
			!strings.HasSuffix(e.differenze[0], "l'utente della fotografia non è fra gli utenti letti: la sigla non si confronta con niente") {
			t.Errorf("%s, con -dsn: differenze %q, non fatti %q", nome, e.differenze, e.nonFatti)
		}
	}
	// Con gli export la riga è quella dell'export: deciso_da non c'è, e resta non confrontabile.
	if e := c5(func(a string) string {
		return sostituisci(t, a, "      nell_export_proposte:\n        stato: confermata\n", "      nell_export_proposte:\n        stato: confermata\n        deciso_da: ACM\n")
	}, f, contestoCorsa{export: true}); len(e.differenze) != 0 || !conPrefisso(e.nonFatti, "decisioni_successive_da_rivedere.file_confermati[0]: proposta.deciso_da (non è negli export)") {
		t.Errorf("con gli export: differenze %q, non fatti %q", e.differenze, e.nonFatti)
	}
	// confermato_da con gli export: la sigla non si confronta, e un campo che l'export non porta nemmeno.
	doc := &fotorfq.DocumentoConfermato{ID: docDaRivedere, ThreadID: threadDecisioni, ConfermatoDa: utenteACME}
	for nome, c := range map[string]struct {
		x    CampoAtteso
		d    *fotorfq.DocumentoConfermato
		non  string
		diff string
	}{
		"la sigla":            {CampoAtteso{Nome: "confermato_da", Valore: "ACM"}, doc, "documento.confermato_da (sigla: gli export non hanno gli utenti", ""},
		"un UUID senza campo": {CampoAtteso{Nome: "confermato_da", Valore: utenteACME.String()}, &fotorfq.DocumentoConfermato{ID: docDaRivedere}, "documento.confermato_da (non è negli export)", ""},
		"un UUID diverso":     {CampoAtteso{Nome: "confermato_da", Valore: uidACME(0x52).String()}, doc, "", "documento.confermato_da: diverso dalla fotografia"},
	} {
		cr := confrontaDocumento([]CampoAtteso{c.x}, c.d, fontiDi(f, ix, true))
		if (c.non == "") != (len(cr.nonConfrontabili) == 0) || (c.non != "" && !strings.HasPrefix(cr.nonConfrontabili[0], c.non)) ||
			(c.diff == "") != (len(cr.differenze) == 0) || (c.diff != "" && cr.differenze[0] != c.diff) {
			t.Errorf("con gli export, %s: %+v", nome, cr)
		}
	}
}

// TestB6cRigheDellAlberoInformative (D-R116 I): le righe della «Conferma l'albero» non sono più fra le parti non
// verificate degli invarianti della C5, che sulla scena sono eseguiti; stanno nella sezione informativa del rapporto,
// con il conteggio e il motivo, nel JSON e nel riepilogo, e la chiusura la mette fra gli informativi incompleti. I loro
// ID il n.2 li risolve ancora: un ID che non c'è resta una differenza.
func TestB6cRigheDellAlberoInformative(t *testing.T) {
	r := corsaBanco(t, mutaBanco{}, true, false)
	if c, _ := controlloBanco(r, ControlloInvariantiC5); c.Stato != ControlloEseguito || c.Differenze != 0 ||
		conPrefisso(dettaglio(r, ControlloInvariantiC5).NonVerificate, "decisioni_successive_da_rivedere.conferma_albero") {
		t.Errorf("gli invarianti della C5: %+v, %+v", c, dettaglio(r, ControlloInvariantiC5))
	}
	a := r.RigheConfermaAlbero
	if a == nil || a.Righe != 2 || !strings.Contains(a.Motivo, "D-R116 I") || strings.Join(a.Percorsi, " ") !=
		"decisioni_successive_da_rivedere.conferma_albero.documenti[0] decisioni_successive_da_rivedere.conferma_albero.allegati[0]" {
		t.Fatalf("la sezione informativa: %+v", a)
	}
	if !strings.Contains(r.Testo(), "\nrighe della «Conferma l'albero» (solo informazione, D-R116 I): 2, non verificate — ") {
		t.Errorf("il riepilogo:\n%s", r.Testo())
	}
	trovata := false
	for _, v := range r.Chiusura.Informativi {
		if v.Nome == SezioneInformativaConfermaAlbero && v.Sede == SedeRapporto && strings.Contains(v.Motivo, "2 righe della «Conferma l'albero» non verificate") {
			trovata = true
		}
	}
	if !trovata {
		t.Errorf("la chiusura: %+v", r.Chiusura.Informativi)
	}
	if righeConfermaAlbero(&SezioniAttesi{}) != nil || righeConfermaAlbero(nil) != nil {
		t.Error("senza righe, nessuna sezione")
	}
	r = corsaBanco(t, attesiMutati(func(a string) string {
		return sostituisci(t, a, "      - documento_id: 00000000-0000-4000-8000-000000000222\n", "      - documento_id: "+uidACME(0x229).String()+"\n")
	}), true, false)
	if !conPrefisso(dettaglio(r, ControlloRisoluzione).Differenze, "decisioni_successive_da_rivedere.conferma_albero.documenti[0].documento_id: non si risolve nella fotografia") {
		t.Errorf("gli ID delle righe dell'albero restano al n.2: %+v", dettaglio(r, ControlloRisoluzione))
	}
}

// TestB6cFinitiFuoriPerimetroConGliExport (D-R116 F): con gli export il predicato dei target confermati senza i finiti
// non si decide, e la parte del n.1 è fuori perimetro: il n.1 è eseguito, con la parte contata nel motivo e scritta nei
// dettagli, e la chiusura elenca la riga dedicata, con la motivazione e il numero delle parti. Una corsa senza quella
// parte non la elenca. Fuori dagli export (la stessa fotografia senza componenti) resterebbe una parte non verificata:
// l'esclusione vale solo lì.
func TestB6cFinitiFuoriPerimetroConGliExport(t *testing.T) {
	in := ingressiACME(t)
	s := sezioniACME(t, nil).Scenario
	esito := esitoConProdotti()
	e := controlloScenario(s, in, true, esito, contestoCorsa{export: true, componentiAssenti: true})
	if c := e.controllo(); c.Stato != ControlloEseguito || c.Motivo != "1 parti fuori perimetro, nei dettagli" || len(e.nonFatti) != 0 ||
		len(e.fuoriPerimetro) != 1 || !strings.HasSuffix(e.fuoriPerimetro[0], "(D-R116 F)") {
		t.Errorf("con gli export: %+v, %+v", c, e)
	}
	if e := controlloScenario(s, in, true, esito, contestoCorsa{componentiAssenti: true}); len(e.fuoriPerimetro) != 0 || len(e.nonFatti) != 1 ||
		e.controllo().Stato != ControlloNonEseguito {
		t.Errorf("senza gli export: %+v", e)
	}
	r := corsaBanco(t, mutaBanco{}, true, false)
	var riga *VoceDiChiusura
	for i, v := range r.Chiusura.FuoriPerimetro {
		if v.Nome == ParteFinitiConGliExport {
			riga = &r.Chiusura.FuoriPerimetro[i]
		}
	}
	if riga == nil || !strings.Contains(riga.Fonte, "D-R116 F") || riga.Motivo != "1 parti di runner_1_scenario_e_casi fuori perimetro in questa corsa" ||
		!strings.Contains(r.Testo(), "runner_1_scenario_e_casi, fuori perimetro: scenario_inoltro.prodotti_confermati_db") ||
		!strings.Contains(r.Testo(), "runner_1_scenario_e_casi: eseguito [obbligatorio, passato] — 1 parti fuori perimetro, nei dettagli") {
		t.Errorf("la chiusura: %+v\n%s", r.Chiusura.FuoriPerimetro, r.Testo())
	}
	b := &banco{r: RapportoBanco{Controlli: controlliPassatiACME()}}
	b.chiudi()
	for _, v := range b.r.Chiusura.FuoriPerimetro {
		if v.Nome == ParteFinitiConGliExport {
			t.Errorf("senza la parte nella corsa la riga non c'è: %+v", b.r.Chiusura.FuoriPerimetro)
		}
	}
}

// TestB6cClientiDegliExportFuoriPerimetro (D-R115 A): con i clienti di un altro DB il controllo è fuori perimetro, con
// la non coerenza degli identificativi nel motivo; lo stato resta NON ESEGUITO e l'uscita 3, anche con -gate; la
// chiusura lo mette fra i fuori perimetro, non fra gli obbligatori non passati.
func TestB6cClientiDegliExportFuoriPerimetro(t *testing.T) {
	altro := mutaBanco{righe: func(r map[string][]riga) {
		for _, c := range r[ExportClienti] {
			if c["cliente_id"] == clienteACME.String() {
				c["cliente_id"] = uidACME(0xac08).String()
			}
		}
	}}
	for _, gate := range []bool{false, true} {
		r := corsaBanco(t, altro, true, gate)
		c, _ := controlloBanco(r, ControlloClientiExport)
		if c.Stato != ControlloNonEseguito || c.Classe != ClasseFuoriPerimetro || c.Esito != TriStatoNonEseguito ||
			c.Motivo != "identificativi dei clienti non coerenti fra gli export: 2 thread esportati su 2 hanno un cliente che l'export dei clienti non contiene, e non si valutano (T-B6-104); limite accettato del campione, fuori perimetro, l'uscita resta 3 (D-R115 A)" {
			t.Errorf("-gate %t: %+v", gate, c)
		}
		if r.Esito.CodiceUscita() != UscitaNonEseguito || r.Differenze != 0 || !strings.Contains(r.PrimaRiga(), "clienti_degli_export: identificativi dei clienti non coerenti") {
			t.Errorf("-gate %t: l'uscita resta 3, con la spiegazione: %s", gate, r.PrimaRiga())
		}
		fuori := false
		for _, v := range r.Chiusura.FuoriPerimetro {
			fuori = fuori || (v.Nome == ControlloClientiExport && v.Esito == TriStatoNonEseguito && strings.Contains(v.Fonte, "D-R115 A"))
		}
		for _, v := range r.Chiusura.Obbligatori.NonPassati {
			if v.Nome == ControlloClientiExport {
				fuori = false
			}
		}
		if !fuori || r.Chiusura.RapportoCompleto {
			t.Errorf("-gate %t: la chiusura: %+v", gate, r.Chiusura)
		}
	}
}

// TestB6cChiusuraConUnFuoriPerimetroNonEseguito (D-R115 A nella chiusura): tutto il resto passato, gli esterni e il gate
// superati, e clienti_degli_export NON ESEGUITO perché fuori perimetro. La parte obbligatoria del runner è conclusa (il
// controllo non è fra i suoi obbligatori), ma il rapporto non è completo e l'uscita resta 3: l'esclusione motivata non
// diventa un successo.
func TestB6cChiusuraConUnFuoriPerimetroNonEseguito(t *testing.T) {
	g := calcolaGate(ingressoGateACME())
	tutte := Gate{Esito: GateSuperato}
	for _, v := range g.Voci {
		v.Stato = VoceSuperata
		tutte.Voci = append(tutte.Voci, v)
	}
	classificaVoci(tutte.Voci)
	cc := controlliPassatiACME()
	for i := range cc {
		if cc[i].Nome == ControlloClientiExport {
			cc[i].Stato, cc[i].Motivo = ControlloNonEseguito, "identificativi dei clienti non coerenti fra gli export"
		}
	}
	b := &banco{r: RapportoBanco{Controlli: cc, Gate: &tutte}}
	b.chiudi()
	ch := b.r.Chiusura
	if ch.Conclusione != ConclusioneConclusa || ch.RapportoCompleto || b.r.Esito.CodiceUscita() != UscitaNonEseguito {
		t.Errorf("chiusura %+v, uscita %d", ch, b.r.Esito.CodiceUscita())
	}
	if txt := b.r.Testo(); !strings.Contains(txt, "\nchiusura: "+ConclusioneConclusa+"; rapporto incompleto; gate: superato\n") ||
		!strings.Contains(txt, "fuori perimetro: clienti_degli_export (non_eseguito)") {
		t.Errorf("il riepilogo:\n%s", txt)
	}
	for i := range cc {
		if cc[i].Nome == ControlloClientiExport {
			cc[i].Stato, cc[i].Motivo = ControlloEseguito, ""
		}
	}
	b = &banco{r: RapportoBanco{Controlli: cc, Gate: &tutte}}
	b.chiudi()
	if !b.r.Chiusura.RapportoCompleto {
		t.Errorf("tutto passato: il rapporto è completo: %+v", b.r.Chiusura)
	}
}

// TestB6cLaBaseDeiRealiDalNome (D-R116 O, la verifica minima): la base dei reali è quella del nome dell'archivio (il
// piano: la forma nome-token su nome_file, gli archivi senza fatti), e il nome si legge per intero: con
// l'interpretazione parziale la base si giudica sulle sole letture del nome (prima era una parte non verificata, «un
// risultato tagliato non si giudica»). Senza letture del nome resta non verificata; una chiave di unicità del file intero
// resta non verificata con il documento parziale; fuori dai reali la base non ha ambito, come prima. token_00_conservato
// è token_conservato (il piano): si giudica sul nome, dove l'ambito lo dà la base. Da C-04 vale solo quando il nome è
// tutto il file (qui il documento ha le sole unità delle sue letture); i casi in cui non lo è sono in
// TestB6cLaBaseDeiRealiSoloSeIlNomeETuttoIlFile.
func TestB6cLaBaseDeiRealiDalNome(t *testing.T) {
	ins := insiemeACME(t)
	lettura := func(unita string, ctx evidenze.Contesto, base, token string) motorea.LetturaCodice {
		l := motorea.LetturaCodice{UnitaID: unita, Funzione: motorea.FunzIdentitaFile,
			Forma: motorea.LetturaForma{Famiglia: "acme-marcatore", Selettore: evidenze.Selettore{Contesto: ctx},
				Base: motorea.BaseLetta{Normalizzata: base, Completa: true}}}
		if token != "" {
			l.Forma.Token = &motorea.ParteLetta{Valore: token, Originale: token}
		}
		return l
	}
	// Il documento ha le unità delle sue letture e la diagnostica dell'archivio che l'acquisizione non apre: con le sole
	// letture del nome, il nome è tutto il file (C-04).
	parziale := func(letture ...motorea.LetturaCodice) fileDelThread {
		var unita []evidenze.UnitaEvidenza
		for _, l := range letture {
			unita = append(unita, evidenze.UnitaEvidenza{ID: l.UnitaID, Selettore: l.Forma.Selettore, Testo: "9123460_00.7z"})
		}
		return fileDelThread{fi: &valut.FileInterpretato{AllegatoID: allArchivio, Documento: documentoDArchivioACME(unita...),
			Interpretazione: motorea.Interpretazione{ClienteID: clienteACME, Stato: motorea.StatoInterpretazioneParziale, Letture: letture}},
			valutato: true, cliente: clienteACME}
	}
	base := func(v string) []ChiaveAttesa {
		return []ChiaveAttesa{{Chiave: "base", Valore: ValoreAtteso{Tipo: TipoIntero, Testo: v}}}
	}
	fd := parziale(lettura("u:nome", evidenze.ContestoNomeFile, "9123460", "00"))
	if d, nf, _ := lettureDelFileNellaSezione("reali_archivi[0]", confronto.SezioneReali, base("9123460"), fd, &ins, ""); len(d) != 0 || len(nf) != 0 {
		t.Errorf("la base giusta sul nome, con il documento parziale: differenze %q, non fatti %q", d, nf)
	}
	if d, nf, _ := lettureDelFileNellaSezione("reali_archivi[0]", confronto.SezioneReali, base("9999999"), fd, &ins, ""); len(nf) != 0 ||
		len(d) != 1 || !strings.HasPrefix(d[0], "reali_archivi[0].atteso.base: atteso 9999999, ottenuto 9123460") {
		t.Errorf("la base sbagliata: differenze %q, non fatti %q", d, nf)
	}
	// Il documento ha solo l'unità del nome, ma nessuna lettura del nome: la base non si giudica.
	soloNome := parziale()
	soloNome.fi.Documento.Unita = []evidenze.UnitaEvidenza{{ID: "u:nome", Selettore: evidenze.Selettore{Contesto: evidenze.ContestoNomeFile}, Testo: "archivio.7z"}}
	if d, nf, _ := lettureDelFileNellaSezione("reali_archivi[0]", confronto.SezioneReali, base("9123460"), soloNome, &ins, ""); len(d) != 0 ||
		len(nf) != 1 || !strings.Contains(nf[0], "nessuna lettura sull'unità nome_file") {
		t.Errorf("senza letture del nome: differenze %q, non fatti %q", d, nf)
	}
	// Una base letta altrove (il contenuto, che nell'archivio non c'è) non vale per il nome: con un'unità del contenuto il
	// nome non è tutto il file, e la base resta sul file intero, non giudicata con il documento parziale (C-04).
	altrove := parziale(lettura("u:pdf:cartiglio:1", evidenze.ContestoCartiglio, "9123460", ""))
	if d, nf, _ := lettureDelFileNellaSezione("reali_archivi[0]", confronto.SezioneReali, base("9123460"), altrove, &ins, ""); len(d) != 0 ||
		len(nf) != 1 || !strings.Contains(nf[0], "un risultato tagliato non si giudica (A1b-22)") {
		t.Errorf("una base letta solo nel contenuto: differenze %q, non fatti %q", d, nf)
	}
	unicita := []ChiaveAttesa{{Chiave: "numero_codici", Valore: ValoreAtteso{Tipo: TipoIntero, Testo: "1"}}}
	if _, nf, _ := lettureDelFileNellaSezione("reali_archivi[0]", confronto.SezioneReali, unicita, fd, &ins, ""); len(nf) != 1 ||
		!strings.Contains(nf[0], "un risultato tagliato non si giudica (A1b-22)") {
		t.Errorf("una chiave di unicità con il documento parziale: %q", nf)
	}
	if _, nf, _ := lettureDelFileNellaSezione("baseline_decisioni[0]", confronto.SezioneBaseline, base("9123460"), fd, &ins, ""); len(nf) != 1 ||
		!strings.Contains(nf[0], "un risultato tagliato non si giudica (A1b-22)") {
		t.Errorf("fuori dai reali la base non ha ambito: %q", nf)
	}
	token := append(base("9123460"), ChiaveAttesa{Chiave: "token_00_conservato", Valore: ValoreAtteso{Tipo: TipoStringa, Testo: "00"}})
	if d, nf, dedotti := lettureDelFileNellaSezione("reali_archivi[0]", confronto.SezioneReali, token, fd, &ins, ""); len(d) != 0 || len(nf) != 0 ||
		len(dedotti) != 1 || !strings.HasPrefix(dedotti[0], "reali_archivi[0].atteso.token_00_conservato: giudicata sull'unità nome_file") {
		t.Errorf("token_00_conservato sul nome: differenze %q, non fatti %q, dedotti %q", d, nf, dedotti)
	}
	senzaToken := parziale(lettura("u:nome", evidenze.ContestoNomeFile, "9123460", ""))
	if d, _, _ := lettureDelFileNellaSezione("reali_archivi[0]", confronto.SezioneReali, token, senzaToken, &ins, ""); len(d) != 1 ||
		!strings.HasPrefix(d[0], "reali_archivi[0].atteso.token_00_conservato: atteso 00") {
		t.Errorf("token_00_conservato senza il token nel nome: %q", d)
	}
	// Nella corsa sulla scena: l'archivio dei reali ha il documento parziale (gli export ACME non hanno fatti), e la
	// base si giudica sul nome: giusta passa, sbagliata è una differenza, mai una parte non verificata.
	r := corsaBanco(t, mutaBanco{}, true, false)
	if conPrefisso(dettaglio(r, ControlloLetture).NonVerificate, "reali_archivi[0].atteso.base") || conPrefisso(dettaglio(r, ControlloLetture).Differenze, "reali_archivi[0].atteso.base") {
		t.Errorf("la base giusta dell'archivio sulla scena: %+v", dettaglio(r, ControlloLetture))
	}
}

// TestB6cLaBaseDeiRealiSoloSeIlNomeETuttoIlFile (C-04; D-2, decisa dall'utente il 10/10; D-R116 O): la base dei reali si
// giudica sul nome solo se il nome è tutto il file, cioè ogni unità del documento è del nome, nessuna è troncata e
// nessun limite rende incompleta la lettura. La sola presenza di una base nel nome non basta: con un contenuto letto
// anche solo in parte, o con un limite superato, la base resta sul file intero e, con l'interpretazione parziale, NON
// VERIFICATA con il motivo di A1b-22. Né la base giusta né quella sbagliata si giudicano lì: nessun passato su una
// lettura parziale.
func TestB6cLaBaseDeiRealiSoloSeIlNomeETuttoIlFile(t *testing.T) {
	// I codici che il banco scrive come testo (di core/estrazione usa solo DaTesto, F19) sono quelli di estrazione.
	if codiceNomeForseTroncato != estrazione.CodiceNomeForseTroncato || codiceArchivioNonEstraibile != estrazione.CodiceArchivioNonEstraibile {
		t.Fatalf("i codici del banco non sono quelli di estrazione: %q %q", codiceNomeForseTroncato, codiceArchivioNonEstraibile)
	}
	ins := insiemeACME(t)
	nome := evidenze.UnitaEvidenza{ID: "u:nome", Selettore: evidenze.Selettore{Contesto: evidenze.ContestoNomeFile}, Testo: "9123460_00.7z"}
	contenuto := evidenze.UnitaEvidenza{ID: "u:pdf:cartiglio:1", Selettore: evidenze.Selettore{Contesto: evidenze.ContestoCartiglio}, Testo: "9123461"}
	troncato := contenuto
	troncato.Qualita.Troncata = true
	nomeTroncato := nome
	nomeTroncato.Qualita.Troncata = true
	lettura := motorea.LetturaCodice{UnitaID: "u:nome", Funzione: motorea.FunzIdentitaFile,
		Forma: motorea.LetturaForma{Famiglia: "acme-marcatore", Selettore: nome.Selettore, Base: motorea.BaseLetta{Normalizzata: "9123460", Completa: true}}}
	limite := []evidenze.Diagnostica{{Codice: grammatica.CodiceLimiteSuperato, Gravita: evidenze.GravitaAvviso, Natura: evidenze.NaturaLimite,
		Percorso: "limiti.riconoscimento.max_unita_documento"}}
	file := func(unita []evidenze.UnitaEvidenza, diag []evidenze.Diagnostica) fileDelThread {
		return fileDelThread{fi: &valut.FileInterpretato{AllegatoID: allArchivio, Documento: documentoDArchivioACME(unita...),
			Interpretazione: motorea.Interpretazione{ClienteID: clienteACME, Stato: motorea.StatoInterpretazioneParziale,
				Letture: []motorea.LetturaCodice{lettura}, Diagnostiche: diag}}, valutato: true, cliente: clienteACME}
	}
	base := func(v string) []ChiaveAttesa {
		return []ChiaveAttesa{{Chiave: "base", Valore: ValoreAtteso{Tipo: TipoIntero, Testo: v}}}
	}
	// Il contenuto non c'è per costruzione solo nell'archivio che l'acquisizione non apre: senza la sua diagnostica, o con
	// la fonte in errore, il solo nome fra le unità vuol dire un contenuto non letto. Il nome che l'acquisizione può aver
	// tagliato (nome.forse_troncato) vale come un'unità troncata.
	senzaArchivio := func(fd fileDelThread) fileDelThread {
		fd.fi.Documento.Qualita.Diagnostiche = nil
		return fd
	}
	inErrore := func(fd fileDelThread) fileDelThread {
		fd.fi.Documento.Qualita.Stato = "errore"
		return fd
	}
	forseTroncato := func(fd fileDelThread) fileDelThread {
		fd.fi.Documento.Qualita.Diagnostiche = append(fd.fi.Documento.Qualita.Diagnostiche, evidenze.Diagnostica{
			Codice: estrazione.CodiceNomeForseTroncato, Gravita: evidenze.GravitaNota, Natura: evidenze.NaturaDati, Percorso: "nome_file"})
		return fd
	}
	for _, c := range []struct {
		nome      string
		fd        fileDelThread
		giudicata bool
		motivo    string
	}{
		{"solo l'unità del nome", file([]evidenze.UnitaEvidenza{nome}, nil), true, ""},
		{"il nome e un'unità del contenuto troncata", file([]evidenze.UnitaEvidenza{nome, troncato}, nil), false, "il documento ha unità oltre al nome (cartiglio)"},
		{"un limite superato", file([]evidenze.UnitaEvidenza{nome}, limite), false, "un limite rende incompleta la lettura (limite.superato)"},
		{"il nome e un'unità del contenuto letta in parte", file([]evidenze.UnitaEvidenza{nome, contenuto}, nil), false, "il documento ha unità oltre al nome"},
		{"l'unità del nome troncata", file([]evidenze.UnitaEvidenza{nomeTroncato}, nil), false, "un'unità del documento è troncata"},
		{"un documento senza unità", file(nil, nil), false, "il documento non ha unità"},
		{"il solo nome senza la diagnostica dell'archivio", senzaArchivio(file([]evidenze.UnitaEvidenza{nome}, nil)), false,
			"il contenuto del file c'è ma non è stato letto"},
		{"il solo nome con la fonte in errore", inErrore(file([]evidenze.UnitaEvidenza{nome}, nil)), false, "la fonte è in errore"},
		{"il nome forse tagliato dall'acquisizione", forseTroncato(file([]evidenze.UnitaEvidenza{nome}, nil)), false,
			"il nome può essere stato tagliato dall'acquisizione (nome.forse_troncato)"},
	} {
		giusta, nfg, _ := lettureDelFileNellaSezione("reali_archivi[0]", confronto.SezioneReali, base("9123460"), c.fd, &ins, "")
		sbagliata, nfs, _ := lettureDelFileNellaSezione("reali_archivi[0]", confronto.SezioneReali, base("9999999"), c.fd, &ins, "")
		if c.giudicata {
			if len(giusta) != 0 || len(nfg) != 0 || len(nfs) != 0 || len(sbagliata) != 1 || !strings.HasPrefix(sbagliata[0], "reali_archivi[0].atteso.base: atteso 9999999") {
				t.Errorf("%s: si giudica sul nome; giusta %q %q, sbagliata %q %q", c.nome, giusta, nfg, sbagliata, nfs)
			}
			continue
		}
		for _, nf := range [][]string{nfg, nfs} {
			if len(nf) != 1 || !strings.HasPrefix(nf[0], "reali_archivi[0].atteso.base: interpretazione parziale: un risultato tagliato non si giudica (A1b-22)") ||
				!strings.Contains(nf[0], c.motivo) {
				t.Errorf("%s: la base resta non verificata, con il motivo di A1b-22: %q", c.nome, nf)
			}
		}
		if len(giusta) != 0 || len(sbagliata) != 0 {
			t.Errorf("%s: nessun giudizio su una lettura parziale: %q %q", c.nome, giusta, sbagliata)
		}
	}
	// I limiti della fonte e una diagnostica di natura limite nel documento valgono come il limite dell'interpretazione.
	fonte := file([]evidenze.UnitaEvidenza{nome}, nil)
	fonte.fi.Documento.Qualita.Limiti = []string{"un limite inventato della fonte"}
	natura := file([]evidenze.UnitaEvidenza{nome}, nil)
	natura.fi.Documento.Qualita.Diagnostiche = []evidenze.Diagnostica{{Codice: "acme.limite", Natura: evidenze.NaturaLimite}}
	if m := ilNomeETuttoIlFile(fonte.fi); m != "la fonte ha limiti di lettura" {
		t.Errorf("i limiti della fonte: %q", m)
	}
	if m := ilNomeETuttoIlFile(natura.fi); m != "un limite rende incompleta la lettura (acme.limite)" {
		t.Errorf("una diagnostica di natura limite nel documento: %q", m)
	}
	// Con l'interpretazione completa, una base senza l'ambito del nome si giudica sul file intero, come prima di C-04.
	completa := file([]evidenze.UnitaEvidenza{nome, contenuto}, nil)
	completa.fi.Interpretazione.Stato = motorea.StatoInterpretazioneCompleta
	if d, nf, _ := lettureDelFileNellaSezione("reali_archivi[0]", confronto.SezioneReali, base("9123460"), completa, &ins, ""); len(d) != 0 || len(nf) != 0 {
		t.Errorf("l'interpretazione completa si giudica sul file intero: differenze %q, non fatti %q", d, nf)
	}
	// I documenti costruiti come nel banco, con estrazione.DaAllegato: tutti hanno il solo nome fra le unità, ma solo
	// l'archivio .7z ha il contenuto assente per costruzione. Il PDF o lo STEP senza fatti, il PDF che il worker non apre
	// (fonte in errore), i fatti con il solo esito, un file qualsiasi senza fatti: il contenuto c'è e non è stato letto, e
	// la base resta NON VERIFICATA con il motivo di A1b-22. Nemmeno il .7z con il nome di 300 rune, che l'acquisizione può
	// aver tagliato.
	daAllegato := func(nomeFile, payload string) fileDelThread {
		sha := shaACME(0x9e1)
		est := strings.TrimPrefix(filepath.Ext(nomeFile), ".")
		a := fotorfq.Allegato{ID: uidACME(0x9e1), MessaggioID: msgScenario, Indice: 1, NomeFile: nomeFile, Estensione: &est,
			Natura: "file", Origine: "outlook", Sha256: &sha}
		var fatti *fotorfq.Fatti
		if payload != "" {
			fatti = &fotorfq.Fatti{Sha256: sha, Payload: []byte(payload)}
		}
		doc, err := estrazione.DaAllegato(a, fatti, nil)
		if err != nil {
			t.Fatalf("%s: %v", nomeFile, err)
		}
		fd := file(nil, nil)
		fd.fi.Documento = doc
		fd.fi.Interpretazione.Letture[0].UnitaID = doc.Unita[0].ID
		return fd
	}
	for _, c := range []struct {
		nome, file, payload string
		giudicata           bool
		motivo              string
	}{
		{"l'archivio .7z", "9123460_00.7z", "", true, ""},
		{"il PDF senza fatti", "9123460_00.pdf", "", false, "il contenuto del file c'è ma non è stato letto"},
		{"il PDF che il worker non apre", "9123460_00.pdf", `{"errore_pdf":"cifrato"}`, false, "la fonte è in errore"},
		{"il PDF con il solo esito", "9123460_00.pdf", `{"esito":{"codice":"ACME"}}`, false, "il contenuto del file c'è ma non è stato letto"},
		{"lo STEP senza fatti", "9123460_00.stp", "", false, "il contenuto del file c'è ma non è stato letto"},
		{"un file qualsiasi senza fatti", "9123460_00.dwg", "", false, "il contenuto del file c'è ma non è stato letto"},
		{"l'archivio .7z con il nome forse tagliato", "9123460_00" + strings.Repeat("x", 287) + ".7z", "", false, "nome.forse_troncato"},
	} {
		fd := daAllegato(c.file, c.payload)
		giusta, nfg, _ := lettureDelFileNellaSezione("reali_archivi[0]", confronto.SezioneReali, base("9123460"), fd, &ins, "")
		sbagliata, nfs, _ := lettureDelFileNellaSezione("reali_archivi[0]", confronto.SezioneReali, base("9999999"), fd, &ins, "")
		if c.giudicata {
			if len(giusta) != 0 || len(nfg) != 0 || len(nfs) != 0 || len(sbagliata) != 1 {
				t.Errorf("%s (DaAllegato): si giudica sul nome; giusta %q %q, sbagliata %q %q", c.nome, giusta, nfg, sbagliata, nfs)
			}
			continue
		}
		for _, nf := range [][]string{nfg, nfs} {
			if len(nf) != 1 || !strings.Contains(nf[0], "un risultato tagliato non si giudica (A1b-22)") || !strings.Contains(nf[0], c.motivo) {
				t.Errorf("%s (DaAllegato): la base resta NON VERIFICATA, con il motivo di A1b-22: %q", c.nome, nf)
			}
		}
		if len(giusta) != 0 || len(sbagliata) != 0 {
			t.Errorf("%s (DaAllegato): nessun giudizio su un contenuto non letto: %q %q", c.nome, giusta, sbagliata)
		}
	}
}

// documentoDArchivioACME: il documento di un archivio che l'acquisizione non apre, con le unità date e la diagnostica
// archivio.non_estraibile che estrazione.DaAllegato gli dà (il contenuto manca per costruzione, C-04).
func documentoDArchivioACME(unita ...evidenze.UnitaEvidenza) evidenze.DocumentoEvidenze {
	return evidenze.DocumentoEvidenze{Unita: unita, Qualita: evidenze.QualitaFonte{Stato: "parziale", Diagnostiche: []evidenze.Diagnostica{{
		Codice: estrazione.CodiceArchivioNonEstraibile, Gravita: evidenze.GravitaAvviso, Natura: evidenze.NaturaCapacita, Percorso: "nome_file"}}}}
}

// TestB6cLaSiglaDellaSezioneC5 (C-05; T-B6c-15, proposta tecnica): deciso_da della sezione C5 non è più descrittiva. Con
// -dsn la sigla deve essere quella di un utente della fotografia (fotorfq.Utenti), e un UUID atteso si confronta come UUID
// con i loro ID: una sigla o un UUID che non corrisponde, anche in una fotografia senza utenti, è una differenza del n.2.
// Con gli export gli utenti non ci sono: la parte è NON VERIFICATA, mai un passato. Nessun SQL nuovo.
func TestB6cLaSiglaDellaSezioneC5(t *testing.T) {
	const p = "decisioni_successive_da_rivedere.deciso_da"
	s, ix := fotoEIndice(t)
	ins := insiemeACME(t)
	dsn := fotoConUtenti(t, s)
	dsn.Origine = fotorfq.OrigineDSN
	// Un secondo utente con lettere esadecimali nell'UUID: il maiuscolo cambia il testo, e un confronto alla lettera
	// dell'UUID non passerebbe (utenteACME ha solo cifre).
	utenteEsaACME := uidACME(0xab)
	if strings.ToUpper(utenteEsaACME.String()) == utenteEsaACME.String() {
		t.Fatalf("l'UUID %s non ha lettere esadecimali", utenteEsaACME)
	}
	dsn.Utenti = append(append([]fotorfq.Utente(nil), dsn.Utenti...), fotorfq.Utente{ID: utenteEsaACME, Sigla: "ABC"})
	senzaUtenti := dsn
	senzaUtenti.Utenti = nil
	export := fotoDellaScena(t, s)
	if export.Origine != fotorfq.OrigineExport || len(export.Utenti) != 0 {
		t.Fatalf("la fotografia degli export della scena: origine %q, utenti %v", export.Origine, export.Utenti)
	}
	n2 := func(valore string, f fotorfq.Fotografia) (differenze, nonFatti []string) {
		sez := sezioniACME(t, func(a string) string {
			return sostituisci(t, formaDellaCorsa(t)(a), "  deciso_da: ACM\n", "  deciso_da: "+valore+"\n")
		})
		e := controlloRisoluzione(sez, traduci(sez, f, ix, &ins), ix, f, false)
		for _, d := range e.differenze {
			if strings.HasPrefix(d, p) {
				differenze = append(differenze, d)
			}
		}
		for _, d := range e.nonFatti {
			if strings.HasPrefix(d, p) {
				nonFatti = append(nonFatti, d)
			}
		}
		return differenze, nonFatti
	}
	for _, c := range []struct {
		nome, valore    string
		f               fotorfq.Fotografia
		differenza, non string
	}{
		{"con -dsn, una sigla presente fra gli utenti passa", "ACM", dsn, "", ""},
		{"con -dsn, una sigla assente è una differenza", "XYZ", dsn, p + ": la sigla non è quella di un utente della fotografia", ""},
		{"con -dsn, una fotografia senza utenti dà una differenza", "ACM", senzaUtenti, p + ": la sigla non è quella di un utente della fotografia", ""},
		{"con gli export la sigla è non verificata", "ACM", export, "", p + ": gli export non hanno gli utenti, e chi ha deciso la sezione non si confronta"},
		{"con -dsn, l'UUID di un utente passa", utenteACME.String(), dsn, "", ""},
		{"con -dsn, l'UUID di un utente con le lettere esadecimali passa, anche in maiuscolo", strings.ToUpper(utenteEsaACME.String()), dsn, "", ""},
		{"con -dsn, l'UUID di nessun utente è una differenza", uidACME(0x52).String(), dsn, p + ": l'UUID non è quello di un utente della fotografia", ""},
		{"con -dsn, la sigla in minuscolo è un'altra sigla", "acm", dsn, p + ": la sigla non è quella di un utente della fotografia", ""},
	} {
		d, nf := n2(c.valore, c.f)
		if strings.Join(d, "|") != c.differenza || strings.Join(nf, "|") != c.non {
			t.Errorf("%s: differenze %q, non fatti %q", c.nome, d, nf)
		}
	}
	// Scritta vuota resta descrittiva: riportata, fra le chiavi libere, senza parti nel n.2.
	vuota := sezioniACME(t, func(a string) string {
		return sostituisci(t, formaDellaCorsa(t)(a), "  deciso_da: ACM\n", "  deciso_da: null\n")
	})
	if vuota.DecisoDaRivedere != nil || !strings.Contains(" "+strings.Join(vuota.ChiaviLibere, " ")+" ", " "+p+" ") {
		t.Errorf("deciso_da vuota: %+v, chiavi libere %v", vuota.DecisoDaRivedere, vuota.ChiaviLibere)
	}
	// Nella corsa sugli export della scena il n.2 la dichiara fra le parti non verificate, senza differenze.
	r := corsaBanco(t, attesiMutati(formaDellaCorsa(t)), true, false)
	if x := dettaglio(r, ControlloRisoluzione); conPrefisso(x.Differenze, p) || !conPrefisso(x.NonVerificate, p+": gli export non hanno gli utenti") {
		t.Errorf("nella corsa sugli export: %+v", x)
	}
}

// chiaviDelRapportoPrimaDiB6c: le chiavi in cima al rapporto versione 3 prima di B6c (QA 1cc6409), e dettaglioPrimaDiB6c
// la forma di un dettaglio di controllo di allora. B6c aggiunge solo campi facoltativi: righe_conferma_albero in cima
// (D-R116 I) e fuori_perimetro nei dettagli (D-R116 F).
var chiaviDelRapportoPrimaDiB6c = []string{"versione_rapporto", "modalita", "esito", "motivo", "differenze", "sorgente", "manifest",
	"sha256_manifest", "sha256_attesi", "sha256_indice", "sha256_casi", "impronta_indice", "versione_limiti", "versione_attesi", "versioni",
	"controlli", "dettagli_controlli", "sola_lettura", "fotografia", "valutazione", "thread", "censimento", "senza_caso",
	"correzioni_manuali", "profilo_limiti", "prodotti", "attesi", "casi", "gate", "chiusura", "scritture"}

type dettaglioPrimaDiB6c struct {
	Nome           string   `json:"nome"`
	Differenze     []string `json:"differenze,omitempty"`
	NonVerificate  []string `json:"non_verificate,omitempty"`
	NonApplicabili []string `json:"non_applicabili,omitempty"`
	AmbitiDedotti  []string `json:"ambiti_dedotti,omitempty"`
}

// TestB6cIlRapportoV3SenzaICampiNuovi (D-6, decisa dall'utente il 10/10: VersioneRapportoBanco resta 3): i campi nuovi
// del rapporto sono facoltativi e, quando mancano, il JSON è quello di prima, byte per byte. Sulla scena ci sono tutti e
// due (la prova li guarda); tolti, le chiavi in cima sono fra quelle di prima, e i dettagli, riletti con la forma di
// prima (che rifiuta un campo in più) e riscritti in forma canonica, sono gli stessi byte. Un lettore della forma di
// prima che ignora i campi sconosciuti legge il rapporto nuovo; nessun campo esistente cambia nome o significato.
func TestB6cIlRapportoV3SenzaICampiNuovi(t *testing.T) {
	if VersioneRapportoBanco != 3 {
		t.Fatalf("la versione del rapporto è %d: resta 3 (D-6)", VersioneRapportoBanco)
	}
	r := corsaBanco(t, mutaBanco{}, true, false)
	js, err := jsoncanonico.Codifica(r)
	if err != nil {
		t.Fatal(err)
	}
	conFuori := false
	for _, d := range r.Dettagli {
		conFuori = conFuori || len(d.FuoriPerimetro) > 0
	}
	if !bytes.Contains(js, []byte(`"righe_conferma_albero":`)) || !conFuori {
		t.Fatalf("la scena non ha i campi nuovi: la prova non li guarda")
	}
	r.RigheConfermaAlbero = nil
	var dettagli []DettaglioControllo
	for _, d := range r.Dettagli {
		d.FuoriPerimetro = nil
		// come banco.esito: un dettaglio senza nessuna parte non si scrive
		if len(d.Differenze)+len(d.NonVerificate)+len(d.NonApplicabili)+len(d.AmbitiDedotti) > 0 {
			dettagli = append(dettagli, d)
		}
	}
	r.Dettagli = dettagli
	if js, err = jsoncanonico.Codifica(r); err != nil {
		t.Fatal(err)
	}
	var parti map[string]json.RawMessage
	if err := json.Unmarshal(js, &parti); err != nil {
		t.Fatal(err)
	}
	prima := " " + strings.Join(chiaviDelRapportoPrimaDiB6c, " ") + " "
	for k := range parti {
		if !strings.Contains(prima, " "+k+" ") {
			t.Errorf("senza i campi nuovi il rapporto ha la chiave %q, che prima non c'era", k)
		}
	}
	if string(parti["versione_rapporto"]) != "3" {
		t.Errorf("versione_rapporto: %s", parti["versione_rapporto"])
	}
	dec := json.NewDecoder(bytes.NewReader(parti["dettagli_controlli"]))
	dec.DisallowUnknownFields()
	var dd []dettaglioPrimaDiB6c
	if err := dec.Decode(&dd); err != nil || len(dd) == 0 {
		t.Fatalf("i dettagli senza i campi nuovi, con la forma di prima: %v (%d)", err, len(dd))
	}
	if vecchi, err := jsoncanonico.Codifica(dd); err != nil || !bytes.Equal(vecchi, parti["dettagli_controlli"]) {
		t.Errorf("i byte dei dettagli non sono quelli della forma di prima:\n%s\n%s", parti["dettagli_controlli"], vecchi)
	}
}

// TestB6cRevisioneSeparata: revisione_confrontata_separatamente contro la riga di confronto del file.
func TestB6cRevisioneSeparata(t *testing.T) {
	riga := map[uuid.UUID]confronto.IndicatoreRevisione{allBaseline: {Valore: confronto.RevisioneNonDeterminabile}}
	if x := revisioneSeparata("p", true, riga, allBaseline); x != "" {
		t.Errorf("con l'indicatore: %q", x)
	}
	if x := revisioneSeparata("p", true, nil, allBaseline); !strings.Contains(x, "non ha una riga di confronto") {
		t.Errorf("senza riga: %q", x)
	}
	if x := revisioneSeparata("p", true, map[uuid.UUID]confronto.IndicatoreRevisione{allBaseline: {}}, allBaseline); !strings.Contains(x, "fuori elenco") {
		t.Errorf("senza valore: %q", x)
	}
	if x := revisioneSeparata("p", false, riga, allBaseline); !strings.Contains(x, "falso") {
		t.Errorf("atteso falso: %q", x)
	}
}
