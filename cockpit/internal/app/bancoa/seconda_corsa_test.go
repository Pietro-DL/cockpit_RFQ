package bancoa

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/confronto"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/platform/dataset"
)

// L1 — ciò che la seconda corsa vera del banco sui dati reali (10/10) chiede alla traduzione, e che le prove sintetiche
// di B6c non vedevano:
//   - gli invarianti della voce della C5 scritti dentro la mappa «atteso» si leggono come invarianti della voce
//     (T-B6c-01, ora anche per la C5); la stessa chiave nella voce e in atteso resta un errore;
//   - fonte_quantita «righe HTML verificate» si traduce come «cella»; un altro valore resta un errore di traduzione;
//   - un export del triage senza la colonna creato_il si legge solo se ogni messaggio ha al più un triage
//     deterministico (la sezione è parziale, con il motivo); con più triage per messaggio, o con un NULL nella colonna
//     presente, l'export non è valido;
//   - con -exports componente_id_proposta_vuoto_preservato della C5 è una parte non applicabile («si verifica con
//     -dsn»); con -dsn resta confrontato con la proposta della fotografia;
//   - gli sha delle fonti degli attesi si leggono anche da sha256_file (non più una chiave libera), e la fonte con lo
//     sha della voce di ruolo dump del manifest va fra le parti non applicabili del n.3, non fra le differenze.
//
// E ciò che la revisione indipendente A6.4 ha chiesto dopo:
//   - un elenco non vuoto di soli valori semplici in una sezione a forma libera è una chiave libera dichiarata (prima
//     spariva in silenzio);
//   - la voce del gate delle forme degli altri profili è fuori perimetro (D-M22, decisa dall'utente il 10/10, R34 c = B):
//     resta non eseguita, il gate resta incompleto, l'uscita con -gate resta 3 e il rapporto non è completo.
//
// Ogni prova fissa una correzione: tolta la correzione, la prova cade (le mutazioni con -overlay sono nello scratchpad
// dell'agente, non nel repository). Le chiavi hanno i nomi del codice; nessun valore degli attesi veri sta qui.
//
// I clienti di questi test sono inventati: vedi scena_banco_test.go.

// invariantiC5NellaVoce: i cinque invarianti della voce della C5 com'è scritta in testdata/attesi_acme.yaml.
const invariantiC5NellaVoce = "      sovrascritta_dal_motore: false\n      componente_id_proposta_vuoto_preservato: false\n" +
	"      in_baseline: false\n      conta_nel_gate: false\n      mostrata_con_badge_di_confronto: true\n"

// c5InAtteso: gli attesi ACME con i cinque invarianti della voce della C5 spostati dentro la mappa «atteso», come li
// ha scritti la seconda corsa.
func c5InAtteso(t *testing.T) func(string) string {
	return func(a string) string {
		return sostituisci(t, a, invariantiC5NellaVoce, "      atteso:\n"+
			"        sovrascritta_dal_motore: false\n        componente_id_proposta_vuoto_preservato: false\n"+
			"        in_baseline: false\n        conta_nel_gate: false\n        mostrata_con_badge_di_confronto: true\n")
	}
}

// TestSecondaCorsaInvariantiDellaC5InAtteso (T-B6c-01 per la C5): i cinque invarianti della voce scritti in atteso si
// leggono come se fossero nella voce, con gli stessi valori: nessuna lettura, nessuna chiave non tradotta, nessun
// errore; nella corsa sugli export la C5 non ha differenze, e un invariante sbagliato in atteso è una differenza come
// nella voce. La stessa chiave nella voce e in atteso è un errore: un valore solo.
func TestSecondaCorsaInvariantiDellaC5InAtteso(t *testing.T) {
	nellaVoce := sezioniACME(t, nil).DaRivedere[0]
	s := sezioniACME(t, c5InAtteso(t))
	if len(s.NonTradotte) != 0 || len(s.Errori) != 0 {
		t.Fatalf("gli invarianti della C5 in atteso: non tradotte %v, errori %v", s.NonTradotte, s.Errori)
	}
	c5 := s.DaRivedere[0]
	if len(c5.Letture) != 0 || c5.Atteso != "" {
		t.Errorf("gli invarianti in atteso non sono letture né l'esito del file: letture %+v, atteso %q", c5.Letture, c5.Atteso)
	}
	for _, k := range []string{"componente_id_proposta_vuoto_preservato", "sovrascritta_dal_motore", "in_baseline",
		"conta_nel_gate", "mostrata_con_badge_di_confronto"} {
		v, ok := c5.invariante(k)
		w, okVoce := nellaVoce.invariante(k)
		if !ok || !okVoce || v != w {
			t.Errorf("%s: in atteso %t (letto %t), nella voce %t (letto %t)", k, v, ok, w, okVoce)
		}
	}
	if len(c5.Invarianti) != len(nellaVoce.Invarianti) {
		t.Errorf("gli invarianti: in atteso %+v, nella voce %+v", c5.Invarianti, nellaVoce.Invarianti)
	}

	// Nella corsa sugli export: nessuna differenza, e le parti non applicabili della C5 sono quelle della scena.
	r := corsaBanco(t, attesiMutati(c5InAtteso(t)), true, false)
	if r.Differenze != 0 {
		t.Fatalf("gli invarianti della C5 in atteso: %s\n%s", r.PrimaRiga(), r.Testo())
	}
	if c, _ := controlloBanco(r, ControlloInvariantiC5); c.Stato != ControlloEseguito || c.Differenze != 0 ||
		strings.Join(dettaglio(r, ControlloInvariantiC5).NonApplicabili, "|") != strings.Join(nonApplicabiliACME, "|") {
		t.Errorf("gli invarianti della C5: %+v, %+v", c, dettaglio(r, ControlloInvariantiC5))
	}
	// Un invariante sbagliato, scritto in atteso, è una differenza della C5.
	for nome, c := range map[string][2]string{
		"il badge":            {"        mostrata_con_badge_di_confronto: true\n", "        mostrata_con_badge_di_confronto: false\n"},
		"conta_nel_gate vero": {"        conta_nel_gate: false\n", "        conta_nel_gate: true\n"},
		"in_baseline vero":    {"        in_baseline: false\n", "        in_baseline: true\n"},
	} {
		r := corsaBanco(t, attesiMutati(func(a string) string { return sostituisci(t, c5InAtteso(t)(a), c[0], c[1]) }), true, false)
		if x, _ := controlloBanco(r, ControlloInvariantiC5); x.Differenze != 1 ||
			!conPrefisso(dettaglio(r, ControlloInvariantiC5).Differenze, "decisioni_successive_da_rivedere.file_confermati[0]: ") {
			t.Errorf("%s in atteso: %+v, %q", nome, x, dettaglio(r, ControlloInvariantiC5).Differenze)
		}
	}
	// La stessa chiave nella voce e in atteso: un errore, con il percorso della voce.
	doppia := sezioniACME(t, func(a string) string {
		return sostituisci(t, c5InAtteso(t)(a), "      atteso:\n        sovrascritta_dal_motore", "      in_baseline: false\n      atteso:\n        sovrascritta_dal_motore")
	})
	if strings.Join(doppia.Errori, "|") != "decisioni_successive_da_rivedere.file_confermati[0].in_baseline: chiave ripetuta, nella voce e in atteso" {
		t.Errorf("la stessa chiave nella voce e in atteso: errori %v", doppia.Errori)
	}
}

// TestSecondaCorsaFonteQuantitaRigheHTML: «righe HTML verificate» è la quantità presa dalle righe della tabella HTML
// della mail, che il motore legge come una cella: si traduce come «cella» (QuantitaDaCella vero). Un altro valore, anche
// vicino, non si inventa: resta un errore di traduzione. Nella corsa sugli export il prodotto della scena con quel
// valore passa come con «cella».
func TestSecondaCorsaFonteQuantitaRigheHTML(t *testing.T) {
	q := 5
	for _, c := range []struct {
		fonte  string
		cella  bool
		errore string
	}{
		{"cella", true, ""},
		{"righe HTML verificate", true, ""},
		{"", false, ""},
		{"righe HTML", false, `fonte_quantita "righe HTML" non tradotta`},
		{"Righe HTML verificate", false, `fonte_quantita "Righe HTML verificate" non tradotta`},
		{"righe HTML verificate dall'utente", false, `fonte_quantita "righe HTML verificate dall'utente" non tradotta`},
	} {
		pa, errore := prodottoAttesoDi(ProdottoScenario{Originale: "P9123456", Base: "9123456", Quantita: &q, FonteQuantita: c.fonte})
		if errore != c.errore {
			t.Errorf("%q: errore %q, atteso %q", c.fonte, errore, c.errore)
		}
		if c.errore == "" && (pa.QuantitaDaCella != nil) != c.cella {
			t.Errorf("%q: QuantitaDaCella %v, attesa %t", c.fonte, pa.QuantitaDaCella, c.cella)
		}
		if c.cella && (pa.QuantitaDaCella == nil || !*pa.QuantitaDaCella) {
			t.Errorf("%q: la quantità dalla cella: %+v", c.fonte, pa)
		}
	}

	conFonte := func(v string) mutaBanco {
		return attesiMutati(func(a string) string {
			return sostituisci(t, a, "      fonte_quantita: cella\n", "      fonte_quantita: "+v+"\n")
		})
	}
	r := corsaBanco(t, conFonte("righe HTML verificate"), true, false)
	if c, _ := controlloBanco(r, ControlloTraduzione); r.Differenze != 0 || c.Differenze != 0 {
		t.Errorf("«righe HTML verificate» nella corsa: %+v, %s\n%s", c, r.PrimaRiga(), r.Testo())
	}
	r = corsaBanco(t, conFonte("righe HTML"), true, false)
	if c, _ := controlloBanco(r, ControlloTraduzione); c.Differenze == 0 ||
		!conPrefisso(dettaglio(r, ControlloTraduzione).Differenze, `scenario_inoltro.prodotti_attesi[0]: fonte_quantita "righe HTML" non tradotta`) ||
		r.Esito.CodiceUscita() != UscitaConDifferenze {
		t.Errorf("un valore non elencato resta un errore di traduzione: %+v, %q", c, dettaglio(r, ControlloTraduzione).Differenze)
	}
}

// fotografiaDegliExport: la fotografia degli export di una scena con FotografiaDaExport, con l'errore (fotoDellaScena
// si ferma su un errore, e qui l'errore è il caso).
func fotografiaDegliExport(t *testing.T, s scenaBanco) (fotorfq.Fotografia, error) {
	t.Helper()
	raw, err := os.ReadFile(s.manifest)
	if err != nil {
		t.Fatal(err)
	}
	m, err := dataset.Leggi(raw, s.dir)
	if err != nil {
		t.Fatal(err)
	}
	return FotografiaDaExport(os.DirFS(s.export), m)
}

// triageDelMessaggio: i triage della fotografia di un messaggio.
func triageDelMessaggio(f fotorfq.Fotografia, msg uuid.UUID) []fotorfq.Triage {
	var out []fotorfq.Triage
	for _, th := range f.Thread {
		for _, x := range th.Triage {
			if x.MessaggioID == msg {
				out = append(out, x)
			}
		}
	}
	return out
}

// TestSecondaCorsaTriageSenzaCreatoIl: creato_il del triage ordina i triage di un messaggio. Un export senza la colonna
// (nessuna riga la porta) si legge se ogni messaggio ha al più un triage deterministico: la sezione del triage è
// parziale, con il motivo accanto a quello delle righe di un'altra fonte escluse. Con più triage deterministici sullo
// stesso messaggio l'ordine non si ricostruisce, e l'export non è valido (nessuna data inventata). La colonna presente
// con un NULL, o presente in una riga e non in un'altra, resta un errore come prima.
func TestSecondaCorsaTriageSenzaCreatoIl(t *testing.T) {
	senzaData := func(r map[string][]riga) {
		for _, x := range r[ExportTriage] {
			delete(x, "creato_il")
		}
	}
	// Un secondo triage deterministico, su un messaggio scelto.
	unAltro := func(n int, msg uuid.UUID) riga {
		return riga{"triage_id": uidACME(n).String(), "messaggio_id": msg.String(), "esito": "nuova_rfq", "atto": nil,
			"legame": nil, "stato": "accettata", "identificativi": "{ACME26-031}", "motivi": "[]", "fonte": "deterministico",
			"deciso_il": istante1}
	}

	// Senza la colonna, un triage deterministico per messaggio (la riga dell'altra fonte è esclusa): si legge.
	f, err := fotografiaDegliExport(t, preparaBanco(t, mutaBanco{righe: senzaData}))
	if err != nil {
		t.Fatalf("senza creato_il, un triage deterministico per messaggio: %v", err)
	}
	st := f.Sezioni[fotorfq.SezioneTriage]
	if st.Stato != fotorfq.StatoSezioneParziale || !strings.Contains(st.Motivo, "senza creato_il: al più un triage deterministico per messaggio") ||
		!strings.Contains(st.Motivo, "solo deterministico: 1 righe di un'altra fonte escluse") {
		t.Errorf("la sezione del triage senza creato_il: %+v", st)
	}
	if x := triageDelMessaggio(f, msgScenario); len(x) != 1 || !x[0].CreatoIl.IsZero() || x[0].DecisoIl == nil {
		t.Errorf("il triage del messaggio dello scenario: %+v", x)
	}
	if d := fotorfq.ValidaFotografia(f); len(d) > 0 {
		t.Errorf("la fotografia senza creato_il del triage non passa ValidaFotografia: %+v", d)
	}
	// Con la colonna la sezione resta quella di prima: filtrata, senza il motivo nuovo.
	if g := fotoDellaScena(t, preparaBanco(t, mutaBanco{})).Sezioni[fotorfq.SezioneTriage]; g.Stato != fotorfq.StatoSezioneFiltrata ||
		strings.Contains(g.Motivo, "creato_il") {
		t.Errorf("con creato_il: %+v", g)
	}
	// Un secondo triage deterministico su un altro messaggio: ancora uno per messaggio, si legge.
	f, err = fotografiaDegliExport(t, preparaBanco(t, mutaBanco{righe: func(r map[string][]riga) {
		r[ExportTriage] = append(r[ExportTriage], unAltro(0x403, msgDecisioni))
		senzaData(r)
	}}))
	if err != nil || f.Sezioni[fotorfq.SezioneTriage].Stato != fotorfq.StatoSezioneParziale || len(triageDelMessaggio(f, msgDecisioni)) != 1 {
		t.Errorf("un triage deterministico su ognuno di due messaggi: %v, %+v", err, f.Sezioni[fotorfq.SezioneTriage])
	}

	for nome, c := range map[string]struct {
		righe func(map[string][]riga)
		parte string
	}{
		"senza la colonna, due triage deterministici sullo stesso messaggio": {func(r map[string][]riga) {
			r[ExportTriage] = append(r[ExportTriage], unAltro(0x403, msgScenario))
			senzaData(r)
		}, `colonna "creato_il": manca, e 1 messaggi hanno più di un triage deterministico: l'ordine per data non si ricostruisce`},
		"la colonna presente con un NULL": {func(r map[string][]riga) {
			r[ExportTriage][0]["creato_il"] = nil
		}, `colonna "creato_il": è NULL`},
		"la colonna presente solo nella riga dell'altra fonte": {func(r map[string][]riga) {
			delete(r[ExportTriage][0], "creato_il")
		}, `colonna "creato_il"`},
	} {
		_, err := fotografiaDegliExport(t, preparaBanco(t, mutaBanco{righe: c.righe}))
		if !errors.Is(err, ErrExportNonValido) || !strings.Contains(err.Error(), c.parte) {
			t.Errorf("%s: %v", nome, err)
		}
	}

	// Nella corsa sugli export della scena, senza la colonna, il banco legge la fotografia: nessuna differenza.
	if r := corsaBanco(t, mutaBanco{righe: senzaData}, true, false); r.Differenze != 0 || r.Esito.CodiceUscita() == UscitaConDifferenze {
		t.Errorf("la corsa senza creato_il: %s\n%s", r.PrimaRiga(), r.Testo())
	}
}

// TestSecondaCorsaComponenteIDConGliExport: componente_id_proposta_vuoto_preservato della C5 descrive la riga della
// proposta del DB, come decisione_proposta. Con -exports è una parte non applicabile («si verifica con -dsn»), qualunque
// sia il valore: mai una differenza, mai un passato. Con -dsn resta confrontato con la proposta della fotografia: il
// valore giusto passa, quello sbagliato è una differenza.
func TestSecondaCorsaComponenteIDConGliExport(t *testing.T) {
	const p = "decisioni_successive_da_rivedere.file_confermati[0]"
	s, ix := fotoEIndice(t)
	f := fotoDellaScena(t, s)
	ins := insiemeACME(t)
	badge := map[uuid.UUID]confronto.Badge{allDaRivedere: confronto.BadgeUguale}
	c5 := func(valore string, cc contestoCorsa) esitoControllo {
		sez := sezioniACME(t, func(a string) string {
			return sostituisci(t, a, "      componente_id_proposta_vuoto_preservato: false\n      in_baseline",
				"      componente_id_proposta_vuoto_preservato: "+valore+"\n      in_baseline")
		})
		return controlloInvariantiC5(&sez, traduci(sez, f, ix, &ins), f, ix, cc, badge)
	}
	// La proposta del file della C5 nella scena ha il componente: il valore giusto è false.
	for _, valore := range []string{"false", "true"} {
		e := c5(valore, contestoCorsa{export: true})
		if len(e.differenze) != 0 || len(e.nonFatti) != 0 ||
			!conPrefisso(e.nonApplicabili, p+".componente_id_proposta_vuoto_preservato: si verifica con -dsn") {
			t.Errorf("con gli export, %s: differenze %q, non fatti %q, non applicabili %q", valore, e.differenze, e.nonFatti, e.nonApplicabili)
		}
	}
	if e := c5("false", contestoCorsa{}); len(e.differenze) != 0 ||
		conPrefisso(e.nonApplicabili, p+".componente_id_proposta_vuoto_preservato") {
		t.Errorf("con -dsn, il valore giusto: differenze %q, non applicabili %q", e.differenze, e.nonApplicabili)
	}
	if e := c5("true", contestoCorsa{}); strings.Join(e.differenze, "|") != p+": componente_id_proposta_vuoto_preservato non coincide" ||
		conPrefisso(e.nonApplicabili, p+".componente_id_proposta_vuoto_preservato") {
		t.Errorf("con -dsn, il valore sbagliato: differenze %q, non applicabili %q", e.differenze, e.nonApplicabili)
	}
	// Con -dsn e una fotografia senza la proposta del file: il valore non si conferma, è una differenza come prima.
	senza := f
	senza.Thread = append([]fotorfq.Thread(nil), f.Thread...)
	for i := range senza.Thread {
		var proposte []fotorfq.PropostaAttuale
		for _, x := range senza.Thread[i].Proposte {
			if x.AllegatoID != allDaRivedere {
				proposte = append(proposte, x)
			}
		}
		senza.Thread[i].Proposte = proposte
	}
	sez := sezioniACME(t, nil)
	if e := controlloInvariantiC5(&sez, traduci(sez, senza, ix, &ins), senza, ix, contestoCorsa{}, badge); !conPrefisso(e.differenze, p+": componente_id_proposta_vuoto_preservato non coincide") {
		t.Errorf("con -dsn, senza la proposta: differenze %q", e.differenze)
	}

	// Nella corsa sugli export il valore sbagliato non dà differenze: la parte è fra le non applicabili della C5.
	r := corsaBanco(t, attesiMutati(func(a string) string {
		return sostituisci(t, a, "      componente_id_proposta_vuoto_preservato: false\n      in_baseline",
			"      componente_id_proposta_vuoto_preservato: true\n      in_baseline")
	}), true, false)
	if c, _ := controlloBanco(r, ControlloInvariantiC5); r.Differenze != 0 || c.Differenze != 0 ||
		!conPrefisso(dettaglio(r, ControlloInvariantiC5).NonApplicabili, p+".componente_id_proposta_vuoto_preservato: si verifica con -dsn") {
		t.Errorf("la corsa sugli export: %+v, %+v", c, dettaglio(r, ControlloInvariantiC5))
	}
}

// shaDumpACME: lo sha256 inventato del dump della scena.
var shaDumpACME = shaACME(0xd0d)

// motivoDumpACME: la parte non applicabile del n.3 per la fonte del dump. Dalla revisione A6.4 il motivo dice chi
// controlla la copia del dump (prima: «si verifica con -dsn»).
var motivoDumpACME = "la fonte " + shaDumpACME + " è il dump del manifest, non un export: il n.3 confronta solo gli export " +
	"(la copia del dump la controllano A1c-L4D-01 e le sentinelle)"

// fontiConShaFile: gli attesi della scena con le fonti degli export scritte come sha256_file (la forma della seconda
// corsa) e una fonte in più, il dump, con il suo sha256_file in maiuscolo. Cambia solo la sezione fonti.
func fontiConShaFile(t *testing.T) func(string) string {
	return func(a string) string {
		i := strings.Index(a, "fonti:\n")
		j := strings.Index(a, "\nprofili:\n")
		if i < 0 || j < i {
			t.Fatalf("gli attesi della scena non hanno la sezione fonti prima dei profili")
		}
		fonti := a[i:j]
		if !strings.Contains(fonti, "    sha256: ") {
			t.Fatalf("la sezione fonti della scena non ha sha256: %q", fonti)
		}
		fonti = strings.ReplaceAll(fonti, "    sha256: ", "    sha256_file: ")
		fonti += "\n  - file: dump_acme.sql\n    sha256_file: " + strings.ToUpper(shaDumpACME)
		return a[:i] + fonti + a[j:]
	}
}

// conVoceDump: il manifest della scena con una voce di ruolo dump (il file non serve alla corsa -exports).
func conVoceDump(m string) string {
	return strings.Replace(m, `"voci": [`, `"voci": [
    {"nome": "dump", "percorso": "dump/dump_acme.sql", "ruolo": "dump", "sha256": "`+shaDumpACME+`", "byte": 0},`, 1)
}

// TestSecondaCorsaFontiConShaFileEIlDump (n.3): gli attesi della seconda corsa scrivono lo sha256 del file di ogni fonte
// come sha256_file, dump compreso. sha256_file è lo stesso valore di sha256, e non è più una chiave libera; il dump si
// riconosce dallo sha della voce di ruolo dump del manifest, ed è una parte non applicabile del n.3 (la copia del dump
// la controllano A1c-L4D-01 e le sentinelle), non una differenza. Una fonte che non è né un export né il dump resta una differenza.
func TestSecondaCorsaFontiConShaFileEIlDump(t *testing.T) {
	sha := strings.Repeat("ab", 32)
	s := sezioniACME(t, func(a string) string {
		return sostituisci(t, a, "fonti: []\n", "fonti:\n  - file: thread_acme.json\n    sha256_file: "+strings.ToUpper(sha)+
			"\n  - file: dump_acme.sql\n    sha256_file: "+shaDumpACME+"\n  - file: messaggi_acme.json\n    sha256: "+shaACME(1)+"\n")
	})
	if len(s.NonTradotte) != 0 || len(s.Errori) != 0 {
		t.Fatalf("le fonti con sha256_file: non tradotte %v, errori %v", s.NonTradotte, s.Errori)
	}
	if strings.Join(s.Fonti, " ") != strings.Join([]string{shaACME(1), shaDumpACME, sha}, " ") {
		t.Errorf("le fonti: %v (sha256 e sha256_file, in minuscolo, ordinate)", s.Fonti)
	}
	for _, k := range s.ChiaviLibere {
		if strings.Contains(k, "sha256") {
			t.Errorf("sha256_file non è una chiave libera: %v", s.ChiaviLibere)
		}
	}
	// sha256_file fuori dalla sezione fonti non è uno sha delle fonti.
	fuori := sezioniACME(t, func(a string) string {
		return sostituisci(t, a, "  - id: integrazione-acme-01\n", "  - id: integrazione-acme-01\n    sha256_file: "+sha+"\n")
	})
	if len(fuori.Fonti) != 0 {
		t.Errorf("sha256_file dei casi di integrazione fra le fonti: %v", fuori.Fonti)
	}

	// controlloFonti: il dump fra le parti non applicabili; senza la voce del dump, una differenza.
	s = sezioniACME(t, nil)
	s.Fonti = []string{shaACME(1), shaACME(2), shaDumpACME}
	e := controlloFonti(&s, []string{shaACME(1), shaACME(2)}, []string{shaDumpACME})
	if c := e.controllo(); c.Stato != ControlloEseguito || c.Differenze != 0 ||
		strings.Join(e.nonApplicabili, "|") != motivoDumpACME {
		t.Errorf("il dump fra le fonti: %+v, non applicabili %q", c, e.nonApplicabili)
	}
	if e := controlloFonti(&s, []string{shaACME(1), shaACME(2)}, nil); len(e.differenze) != 1 || len(e.nonApplicabili) != 0 ||
		!strings.HasSuffix(e.differenze[0], shaDumpACME) {
		t.Errorf("senza la voce del dump: differenze %q, non applicabili %q", e.differenze, e.nonApplicabili)
	}
	// Il dump non toglie la direzione opposta: un export che non è fra le fonti resta una differenza.
	if e := controlloFonti(&s, []string{shaACME(1), shaACME(2), shaACME(3)}, []string{shaDumpACME}); len(e.differenze) != 1 {
		t.Errorf("un export fuori dalle fonti, con il dump: %q", e.differenze)
	}

	// Nella corsa sugli export: le fonti con sha256_file e il dump del manifest, nessuna differenza.
	conDump := mutaBanco{testi: map[string]func(string) string{"attesi": fontiConShaFile(t), "manifest": conVoceDump}}
	r := corsaBanco(t, conDump, true, false)
	if c, _ := controlloBanco(r, ControlloFonti); c.Stato != ControlloEseguito || c.Differenze != 0 || r.Differenze != 0 ||
		strings.Join(dettaglio(r, ControlloFonti).NonApplicabili, "|") != motivoDumpACME {
		t.Errorf("le fonti della seconda corsa: %+v, %+v, %s", c, dettaglio(r, ControlloFonti), r.PrimaRiga())
	}
	// Senza la voce del dump nel manifest, la fonte del dump non è fra gli export: una differenza del n.3.
	r = corsaBanco(t, attesiMutati(fontiConShaFile(t)), true, false)
	if c, _ := controlloBanco(r, ControlloFonti); c.Differenze != 1 || len(dettaglio(r, ControlloFonti).NonApplicabili) != 0 ||
		!conPrefisso(dettaglio(r, ControlloFonti).Differenze, "uno sha256 delle fonti degli attesi non è fra gli export del manifest: "+shaDumpACME) {
		t.Errorf("il dump senza la voce del manifest: %+v, %+v", c, dettaglio(r, ControlloFonti))
	}
}

// TestA64ElenchiDiValoriSempliciFraLeChiaviLibere (revisione A6.4): in una sezione a forma libera (qui i casi di
// integrazione) un elenco non vuoto di soli valori semplici è una chiave libera dichiarata, con «[]» al posto degli
// indici, come un valore semplice; prima non si giudicava e non si dichiarava. Restano come prima: un elenco vuoto non
// dichiara niente, un elenco di mappe dichiara le chiavi delle mappe, una chiave degli ID con un elenco resta fra gli
// ID e non fra le chiavi libere. Nella corsa la chiave arriva fra le chiavi libere non verificate del rapporto.
func TestA64ElenchiDiValoriSempliciFraLeChiaviLibere(t *testing.T) {
	const voce = "  - id: integrazione-acme-01\n"
	libere := func(aggiunta string) SezioniAttesi {
		return sezioniACME(t, func(a string) string { return sostituisci(t, a, voce, voce+aggiunta) })
	}
	della := func(s SezioniAttesi, prefisso string) []string {
		var out []string
		for _, k := range s.ChiaviLibere {
			if strings.HasPrefix(k, prefisso) {
				out = append(out, k)
			}
		}
		return out
	}
	for nome, c := range map[string]struct {
		aggiunta string
		attese   string
	}{
		"un elenco di valori semplici":     {"    marcatori_acme: [A, B]\n", "casi_integrazione[].marcatori_acme"},
		"un elenco di un valore solo":      {"    marcatori_acme:\n      - 1\n", "casi_integrazione[].marcatori_acme"},
		"un elenco con un null":            {"    marcatori_acme: [A, null]\n", "casi_integrazione[].marcatori_acme"},
		"un valore semplice, come prima":   {"    marcatori_acme: A\n", "casi_integrazione[].marcatori_acme"},
		"un elenco vuoto, come prima":      {"    marcatori_acme: []\n", ""},
		"un elenco di mappe, come prima":   {"    marcatori_acme:\n      - lettera: A\n      - lettera: B\n", "casi_integrazione[].marcatori_acme[].lettera"},
		"una mappa con un elenco annidato": {"    marcatori_acme:\n      lettere: [A, B]\n", "casi_integrazione[].marcatori_acme.lettere"},
	} {
		s := libere(c.aggiunta)
		if len(s.NonTradotte) != 0 || len(s.Errori) != 0 {
			t.Errorf("%s: non tradotte %v, errori %v", nome, s.NonTradotte, s.Errori)
		}
		if got := strings.Join(della(s, "casi_integrazione[].marcatori_acme"), "|"); got != c.attese {
			t.Errorf("%s: chiavi libere %q, attese %q", nome, got, c.attese)
		}
	}
	// Una chiave degli ID con un elenco resta fra gli ID, come prima: non è una chiave libera.
	s := libere("    messaggio_id: [" + msgScenario.String() + ", " + msgDecisioni.String() + "]\n")
	if len(della(s, "casi_integrazione[].messaggio_id")) != 0 {
		t.Errorf("messaggio_id con un elenco fra le chiavi libere: %v", s.ChiaviLibere)
	}
	ids := 0
	for _, x := range s.Integrazione {
		if x.Chiave == "messaggio_id" && (x.Valore == msgScenario.String() || x.Valore == msgDecisioni.String()) {
			ids++
		}
	}
	if ids != 2 {
		t.Errorf("gli ID dell'elenco: %+v", s.Integrazione)
	}

	// Nella corsa sugli export: la chiave sta fra le chiavi libere non verificate del rapporto.
	r := corsaBanco(t, attesiMutati(func(a string) string { return sostituisci(t, a, voce, voce+"    marcatori_acme: [A, B]\n") }), true, false)
	if r.Attesi == nil || !strings.Contains(" "+strings.Join(r.Attesi.ChiaviLibere, " ")+" ", " casi_integrazione[].marcatori_acme ") {
		t.Errorf("la corsa: chiavi libere %+v", r.Attesi)
	}
}

// TestA64AltriProfiliFuoriPerimetro (D-M22, decisa dall'utente il 10/10, R34 c = B): la voce del gate delle forme dei
// due profili senza etichette esce dagli obbligatori esterni ed è fuori perimetro, senza prova, con la fonte della
// decisione. Cambia solo la classe: la voce resta non eseguita, mai superata; il gate resta incompleto; con -gate l'uscita
// resta 3; il rapporto non è completo, perché il controllo del gate resta NON ESEGUITO. Nella chiusura la voce sta fra i
// fuori perimetro, non fra gli esterni, dove restano zero_scritture_di_dominio e motore_senza_llm.
func TestA64AltriProfiliFuoriPerimetro(t *testing.T) {
	c := classeDi(SedeGate, VoceGateAltriProfili)
	if c.Classe != ClasseFuoriPerimetro || c.Prova != "" || !strings.Contains(c.Fonte, "D-M22") || c.Nota == "" {
		t.Fatalf("la classe della voce: %+v", c)
	}
	g := calcolaGate(ingressoGateACME())
	v := voce(g, VoceGateAltriProfili)
	if v.Classe != ClasseFuoriPerimetro || v.Stato != VoceNonEseguita || v.Esito != TriStatoNonEseguito || g.Esito != GateIncompleto {
		t.Fatalf("la voce nel gate: %+v, gate %s", v, g.Esito)
	}
	b := &banco{o: Opzioni{Gate: true}, r: RapportoBanco{Controlli: append(controlliPassatiACME(), esitoUscitaGate(g).controllo()), Gate: &g}}
	b.chiudi()
	ch := b.r.Chiusura
	if b.r.Esito.CodiceUscita() != UscitaNonEseguito || ch.RapportoCompleto || ch.Conclusione != ConclusioneConEsterni {
		t.Errorf("con -gate: uscita %d, chiusura %+v", b.r.Esito.CodiceUscita(), ch)
	}
	var esterni []string
	for _, e := range ch.Esterni {
		esterni = append(esterni, e.Nome)
	}
	if strings.Join(esterni, " ") != VoceGateZeroScritture+" "+VoceGateMotoreSenzaLLM {
		t.Errorf("gli esterni: %v", esterni)
	}
	fuori := 0
	for _, f := range ch.FuoriPerimetro {
		if f.Nome == VoceGateAltriProfili {
			fuori++
			if f.Sede != SedeGate || f.Esito != TriStatoNonEseguito || !strings.Contains(f.Fonte, "D-M22") {
				t.Errorf("la voce fra i fuori perimetro: %+v", f)
			}
		}
	}
	if fuori != 1 {
		t.Errorf("la voce non è una volta fra i fuori perimetro: %+v", ch.FuoriPerimetro)
	}

	// La voce da sola tiene il rapporto incompleto: tutte le altre voci superate, lei sola non eseguita (come sempre nel
	// runner). Il controllo del gate è NON ESEGUITO per lei, l'uscita con -gate è 3, la parte obbligatoria è conclusa senza
	// esterni da chiudere, e il rapporto non è completo. Superata (cosa che il runner non fa), il rapporto sarebbe completo.
	chiudi := func(stato string) RapportoBanco {
		x := Gate{}
		for _, w := range g.Voci {
			w.Stato = VoceSuperata
			if w.Nome == VoceGateAltriProfili {
				w.Stato = stato
			}
			x.Voci = append(x.Voci, w)
		}
		classificaVoci(x.Voci)
		x.Esito = GateSuperato
		if stato == VoceNonEseguita {
			x.Esito = GateIncompleto
		}
		b := &banco{o: Opzioni{Gate: true}, r: RapportoBanco{Controlli: append(controlliPassatiACME(), esitoUscitaGate(x).controllo()), Gate: &x}}
		b.chiudi()
		return b.r
	}
	r := chiudi(VoceNonEseguita)
	if cg, _ := controlloBanco(r, nomeControlloGate); cg.Stato != ControlloNonEseguito || !strings.Contains(cg.Motivo, GateIncompleto) {
		t.Errorf("il controllo del gate con la sola voce non eseguita: %+v", cg)
	}
	if r.Esito.CodiceUscita() != UscitaNonEseguito || r.Chiusura.RapportoCompleto || r.Chiusura.Conclusione != ConclusioneConclusa ||
		len(r.Chiusura.Esterni) != 2 {
		t.Errorf("la sola voce fuori perimetro non eseguita: uscita %d, chiusura %+v", r.Esito.CodiceUscita(), r.Chiusura)
	}
	if r := chiudi(VoceSuperata); !r.Chiusura.RapportoCompleto || r.Esito.CodiceUscita() == UscitaNonEseguito {
		t.Errorf("tutte le voci superate: uscita %d, chiusura %+v", r.Esito.CodiceUscita(), r.Chiusura)
	}
}
