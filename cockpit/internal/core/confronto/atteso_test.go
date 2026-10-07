// L1 — gli esiti contro l'atteso (A1c-L1-19; R30 a-e A; la tavola degli esiti del 6.4.6): ConfrontaConAtteso dà ogni
// riga della tavola con il suo peso nel gate, le radici in più e in meno, l'autorità, i riservati, la voce che dipende da
// una domanda aperta (valutata contro il default, annotata, non riservata), le sezioni con i nomi neutri (P-10; M-21);
// ConfrontaProdotti lega i prodotti attesi ai candidati della mail per base. L'atteso arriva come struttura, tradotta
// dal runner: qui la scrive la prova.
//
// I clienti, i codici e gli ID sono inventati (ACME, 712xxxx, UUID 00000000-0000-4000-8000-0000000000nn): il repository
// è pubblico.

package confronto_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"promatec/cockpit/internal/core/confronto"
)

func conRadici(c confronto.Candidato, radici ...string) confronto.Candidato {
	c.Radici = radici
	return c
}

func conAutorita(c confronto.Candidato, a string) confronto.Candidato { c.Autorita = a; return c }

// controUno: ConfrontaConAtteso su un file e una voce.
func controUno(t *testing.T, n confronto.Nuovo, a confronto.FileAtteso, altre bool) confronto.EsitoFileAtteso {
	t.Helper()
	a.AllegatoID = id(1)
	out := confronto.ConfrontaConAtteso([]confronto.File{{AllegatoID: id(1), Nuovo: n}}, confronto.Atteso{ThreadID: id(99),
		File: []confronto.FileAtteso{a}, AltreAmmesse: altre})
	if len(out) != 1 {
		t.Fatalf("esiti %+v", out)
	}
	return out[0]
}

func TestEsitiControAttesoRigaPerRiga(t *testing.T) {
	figlio := func(c ...confronto.Candidato) confronto.Nuovo {
		return valutato("figlio", "candidato_unico", []string{"7120101"}, c...)
	}
	casi := []struct {
		nome   string
		n      confronto.Nuovo
		a      confronto.FileAtteso
		altre  bool
		esito  confronto.EsitoAtteso
		peso   string
		motivo string
	}{
		{"corretto, figlio con le radici attese",
			figlio(conRadici(candidato(1, "7120101"), "7120100", "7120200")),
			confronto.FileAtteso{Atteso: "figlio", TargetBase: "7120101", Radici: []string{"7120200", "7120100"}, Sezione: "scenario"},
			false, confronto.EsitoCorretto, confronto.PesoCopertura, ""},
		{"corretto, radice di scenario con candidato_unico e autorità scenario",
			valutato("radice", "candidato_unico", []string{"7120100"}, confronto.Candidato{Target: "identificativo:P7120100", Livello: "prodotto", Base: "7120100", Autorita: "scenario"}),
			confronto.FileAtteso{Atteso: "radice", TargetBase: "7120100", StatoAtteso: "candidato_scenario", Sezione: "scenario"},
			false, confronto.EsitoCorretto, confronto.PesoCopertura, ""},
		{"ambiguo: alternative conservate con T in C",
			valutato("figlio", "ambiguo", []string{"7120101"}, candidato(1, "7120101"), candidato(2, "7120102")),
			confronto.FileAtteso{Atteso: "figlio", TargetBase: "7120101", Sezione: "scenario"},
			false, confronto.EsitoAmbiguo, confronto.PesoAstensione, ""},
		{"ambiguo: discordante con T in C",
			valutato("figlio", "discordante", []string{"7120101"}, candidato(1, "7120101")),
			confronto.FileAtteso{Atteso: "figlio", TargetBase: "7120101", Sezione: "scenario"},
			false, confronto.EsitoAmbiguo, confronto.PesoAstensione, ""},
		// R-41: un candidato la cui base non si legge sta nei candidati, mai nelle basi attese.
		{"errato: il solo candidato non ha la base",
			valutato("figlio", "candidato_unico", []string{"7120101"}, candidato(1, "")),
			confronto.FileAtteso{Atteso: "figlio", TargetBase: "7120101", Sezione: "baseline"},
			false, confronto.EsitoErrato, confronto.PesoBloccante, confronto.MotivoAttesoBaseFuoriAtteso},
		{"falsa associazione: atteso fuori e un candidato senza base",
			valutato("figlio", "candidato_unico", nil, candidato(1, "")),
			confronto.FileAtteso{Atteso: "fuori", Sezione: "scenario"},
			false, confronto.EsitoFalsaAssociazione, confronto.PesoBloccante, confronto.MotivoAttesoCandidatiConFuori},
		{"falsa associazione: nessun target atteso e un candidato senza base",
			valutato("figlio", "candidato_unico", nil, candidato(1, "")),
			confronto.FileAtteso{Sezione: "baseline"},
			false, confronto.EsitoFalsaAssociazione, confronto.PesoBloccante, confronto.MotivoAttesoCandidatiSenzaTarget},
		{"falsa associazione: un candidato senza base accanto a quello giusto",
			figlio(candidato(1, "7120101"), candidato(2, "")),
			confronto.FileAtteso{Atteso: "figlio", TargetBase: "7120101", Sezione: "scenario"},
			false, confronto.EsitoFalsaAssociazione, confronto.PesoBloccante, confronto.MotivoAttesoCandidatiInPiu},
		// R-42: una nessuna risposta non è mai copertura, nemmeno senza target atteso.
		{"mancante: nessun target atteso e file non valutato",
			confronto.Nuovo{Valutato: false, Motivo: "contenitore", Associazione: "non_valutata"},
			confronto.FileAtteso{Atteso: "radice", Sezione: "baseline"},
			false, confronto.EsitoMancante, confronto.PesoAstensione, confronto.MotivoAttesoNonValutato},
		// R-49: l'ambiguità resta un'astensione, ma il motivo dice le radici in più e l'autorità diversa.
		{"ambiguo, con le radici in più nel motivo",
			valutato("figlio", "ambiguo", []string{"7120101"}, conRadici(candidato(1, "7120101"), "7120100", "7120900"), candidato(2, "7120102")),
			confronto.FileAtteso{Atteso: "figlio", TargetBase: "7120101", Radici: []string{"7120100"}, Sezione: "scenario"},
			false, confronto.EsitoAmbiguo, confronto.PesoAstensione, confronto.MotivoAttesoRadiciInPiu},
		{"ambiguo, con le radici in più e l'autorità diversa nel motivo",
			valutato("figlio", "ambiguo", []string{"7120101"}, conRadici(candidato(1, "7120101"), "7120100", "7120900"), candidato(2, "7120102")),
			confronto.FileAtteso{Atteso: "figlio", TargetBase: "7120101", Radici: []string{"7120100"}, Autorita: "confermata", Sezione: "scenario"},
			false, confronto.EsitoAmbiguo, confronto.PesoAstensione, confronto.MotivoAttesoRadiciInPiu + "," + confronto.MotivoAttesoAutoritaDiversa},
		// R-62: due target raggiunti, uno con la base che non si legge (""): una radice in più, che non sparisce mai e
		// non coincide con nessuna base attesa, nemmeno con un "" fra le attese.
		{"falsa associazione: due radici raggiunte, una con la base illeggibile",
			figlio(conRadici(candidato(1, "7120101"), "7120100", "")),
			confronto.FileAtteso{Atteso: "figlio", TargetBase: "7120101", Radici: []string{"7120100"}, Sezione: "scenario"},
			false, confronto.EsitoFalsaAssociazione, confronto.PesoBloccante, confronto.MotivoAttesoRadiciInPiu},
		{"falsa associazione: la radice illeggibile non coincide con un vuoto fra le attese",
			figlio(conRadici(candidato(1, "7120101"), "7120100", "")),
			confronto.FileAtteso{Atteso: "figlio", TargetBase: "7120101", Radici: []string{"7120100", ""}, Sezione: "scenario"},
			false, confronto.EsitoFalsaAssociazione, confronto.PesoBloccante, confronto.MotivoAttesoRadiciInPiu},
		{"ambiguo, con la radice illeggibile nel motivo",
			valutato("figlio", "ambiguo", []string{"7120101"}, conRadici(candidato(1, "7120101"), "7120100", ""), candidato(2, "7120102")),
			confronto.FileAtteso{Atteso: "figlio", TargetBase: "7120101", Radici: []string{"7120100"}, Sezione: "scenario"},
			false, confronto.EsitoAmbiguo, confronto.PesoAstensione, confronto.MotivoAttesoRadiciInPiu},
		{"errato: nessuna base dei candidati è attesa",
			figlio(candidato(2, "7120102")),
			confronto.FileAtteso{Atteso: "figlio", TargetBase: "7120101", Sezione: "scenario"},
			false, confronto.EsitoErrato, confronto.PesoBloccante, confronto.MotivoAttesoBaseFuoriAtteso},
		{"errato: le basi giuste con un'autorità diversa dall'attesa (R30 e)",
			valutato("radice", "candidato_unico", []string{"7120100"}, confronto.Candidato{Target: "identificativo:P7120100", Livello: "prodotto", Base: "7120100", Autorita: "confermata"}),
			confronto.FileAtteso{Atteso: "radice", TargetBase: "7120100", StatoAtteso: "candidato_scenario", Sezione: "scenario"},
			false, confronto.EsitoErrato, confronto.PesoBloccante, confronto.MotivoAttesoAutoritaDiversa},
		{"errato: autorità attesa dichiarata e diversa",
			figlio(conAutorita(candidato(1, "7120101"), "scenario")),
			confronto.FileAtteso{TargetBase: "7120101", Autorita: "confermata", Sezione: "baseline"},
			false, confronto.EsitoErrato, confronto.PesoBloccante, confronto.MotivoAttesoAutoritaDiversa},
		{"errato: la collocazione non è quella attesa",
			figlio(candidato(1, "7120101")),
			confronto.FileAtteso{Atteso: "radice", TargetBase: "7120101", Sezione: "scenario"},
			false, confronto.EsitoErrato, confronto.PesoBloccante, confronto.MotivoAttesoCollocazioneDiversa},
		{"mancante: nessun candidato",
			valutato("non_determinabile", "nessun_candidato", []string{"7120101"}),
			confronto.FileAtteso{Atteso: "figlio", TargetBase: "7120101", Sezione: "scenario"},
			false, confronto.EsitoMancante, confronto.PesoAstensione, confronto.MotivoAttesoNonDeterminabile},
		{"mancante: file non valutato",
			confronto.Nuovo{Valutato: false, Motivo: "thread_non_valutato", Associazione: "non_valutata"},
			confronto.FileAtteso{Atteso: "figlio", TargetBase: "7120101", Sezione: "scenario"},
			false, confronto.EsitoMancante, confronto.PesoAstensione, confronto.MotivoAttesoNonValutato},
		{"mancante: nessun candidato, collocazione non data",
			valutato("", "nessun_candidato", nil),
			confronto.FileAtteso{TargetBase: "7120101", Sezione: "baseline"},
			false, confronto.EsitoMancante, confronto.PesoAstensione, confronto.MotivoAttesoNessunCandidato},
		{"mancante: atteso fuori e nessuna risposta",
			valutato("non_determinabile", "nessun_candidato", nil),
			confronto.FileAtteso{Atteso: "fuori", Sezione: "scenario"},
			false, confronto.EsitoMancante, confronto.PesoAstensione, confronto.MotivoAttesoNonDeterminabile},
		{"mancante: radici attese non raggiunte (R30 e)",
			figlio(conRadici(candidato(1, "7120101"), "7120100")),
			confronto.FileAtteso{Atteso: "figlio", TargetBase: "7120101", Radici: []string{"7120100", "7120200"}, Sezione: "scenario"},
			false, confronto.EsitoMancante, confronto.PesoAstensione, confronto.MotivoAttesoRadiciMancanti},
		{"mancante: candidato_scenario senza candidato_unico",
			valutato("radice", "non_valutata", []string{"7120100"}, confronto.Candidato{Target: "identificativo:P7120100", Livello: "prodotto", Base: "7120100", Autorita: "scenario"}),
			confronto.FileAtteso{Atteso: "radice", TargetBase: "7120100", StatoAtteso: "candidato_scenario", Sezione: "scenario"},
			false, confronto.EsitoMancante, confronto.PesoAstensione, confronto.MotivoAttesoNonCandidatoUnico},
		{"falsa associazione: atteso fuori e candidati",
			figlio(candidato(1, "7120101")),
			confronto.FileAtteso{Atteso: "fuori", Sezione: "scenario"},
			false, confronto.EsitoFalsaAssociazione, confronto.PesoBloccante, confronto.MotivoAttesoCandidatiConFuori},
		{"falsa associazione: nessun ancoraggio atteso e candidati",
			figlio(candidato(1, "7120101")),
			confronto.FileAtteso{Sezione: "baseline"},
			false, confronto.EsitoFalsaAssociazione, confronto.PesoBloccante, confronto.MotivoAttesoCandidatiSenzaTarget},
		{"falsa associazione: radici in più (R30 e)",
			figlio(conRadici(candidato(1, "7120101"), "7120100", "7120200")),
			confronto.FileAtteso{Atteso: "figlio", TargetBase: "7120101", Radici: []string{"7120100"}, Sezione: "scenario"},
			false, confronto.EsitoFalsaAssociazione, confronto.PesoBloccante, confronto.MotivoAttesoRadiciInPiu},
		{"falsa associazione: una base in più senza altre letture ammesse",
			figlio(candidato(1, "7120101"), candidato(2, "7120102")),
			confronto.FileAtteso{Atteso: "figlio", TargetBase: "7120101", Sezione: "scenario"},
			false, confronto.EsitoFalsaAssociazione, confronto.PesoBloccante, confronto.MotivoAttesoCandidatiInPiu},
		{"corretto: una base in più con le altre letture ammesse",
			figlio(candidato(1, "7120101"), candidato(2, "7120102")),
			confronto.FileAtteso{Atteso: "figlio", TargetBase: "7120101", Sezione: "scenario"},
			true, confronto.EsitoCorretto, confronto.PesoCopertura, ""},
		{"corretto: nessun ancoraggio atteso, nessun candidato",
			valutato("non_determinabile", "nessun_candidato", nil),
			confronto.FileAtteso{Sezione: "baseline"},
			false, confronto.EsitoCorretto, confronto.PesoCopertura, ""},
		{"fuori richiesta: atteso fuori e collocazione fuori_richiesta",
			valutato("fuori_richiesta", "nessun_candidato", []string{"7120500"}),
			confronto.FileAtteso{Atteso: "fuori", Sezione: "scenario"},
			false, confronto.EsitoFuoriRichiesta, confronto.PesoCopertura, ""},
		{"fuori richiesta: stato atteso fuori_scenario",
			valutato("fuori_richiesta", "nessun_candidato", nil),
			confronto.FileAtteso{StatoAtteso: "fuori_scenario", Sezione: "scenario"},
			false, confronto.EsitoFuoriRichiesta, confronto.PesoCopertura, ""},
		{"da rivedere: la sezione C5 vince sul resto",
			figlio(candidato(2, "7120102")),
			confronto.FileAtteso{TargetBase: "7120101", Sezione: "da_rivedere"},
			false, confronto.EsitoDaRivedere, confronto.PesoFuoriDalGate, ""},
		{"riservato, con il motivo",
			figlio(candidato(1, "7120101")),
			confronto.FileAtteso{Atteso: "figlio", TargetBase: "7120101", Sezione: "reali", Riservato: "predicato_non_verificabile"},
			false, confronto.EsitoRiservato, confronto.PesoMaiPassato, "predicato_non_verificabile"},
		{"riservato per lo stato atteso",
			figlio(candidato(1, "7120101")),
			confronto.FileAtteso{TargetBase: "7120101", StatoAtteso: "riservato", Sezione: "scenario"},
			false, confronto.EsitoRiservato, confronto.PesoMaiPassato, confronto.MotivoAttesoStatoRiservato},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			r := controUno(t, c.n, c.a, c.altre)
			if r.Esito != c.esito || r.Peso != c.peso || r.Motivo != c.motivo || r.Sezione != c.a.Sezione || r.AllegatoID != id(1) {
				t.Errorf("esito %+v, attesi %s / %s / %q", r, c.esito, c.peso, c.motivo)
			}
		})
	}
}

// TestEsitoCheDipendeDaUnaDomanda (6.4.6; R30): un caso definito che dipende da una domanda aperta si valuta contro il
// default conservativo dell'atteso, e l'esito lo annota: non è riservato.
func TestEsitoCheDipendeDaUnaDomanda(t *testing.T) {
	r := controUno(t, valutato("figlio", "candidato_unico", nil, candidato(1, "7120101")),
		confronto.FileAtteso{Atteso: "figlio", TargetBase: "7120101", Sezione: "scenario", DipendeDa: "Q1"}, false)
	if r.Esito != confronto.EsitoCorretto || r.DipendeDa != "Q1" {
		t.Errorf("esito %+v", r)
	}
}

// TestEsitiNonCopertiEFileAssenti (R30 d A): un file del thread senza voce è non_coperto, fuori dal gate; una voce senza
// il suo file è mancante; un file con due voci (due sezioni) ha due esiti; l'ordine è per allegato, poi per sezione, e
// non dipende da quello degli ingressi.
func TestEsitiNonCopertiEFileAssenti(t *testing.T) {
	file := []confronto.File{
		{AllegatoID: id(3), Nuovo: valutato("figlio", "candidato_unico", nil, candidato(1, "7120101"))},
		{AllegatoID: id(1), Nuovo: valutato("figlio", "candidato_unico", nil, candidato(1, "7120101"))},
	}
	voci := []confronto.FileAtteso{
		{AllegatoID: id(2), TargetBase: "7120101", Sezione: "scenario"},
		{AllegatoID: id(1), TargetBase: "7120101", Sezione: "scenario", Atteso: "figlio"},
		{AllegatoID: id(1), TargetBase: "7120101", Sezione: "baseline"},
	}
	out := confronto.ConfrontaConAtteso(file, confronto.Atteso{File: voci})
	var righe []string
	for _, r := range out {
		righe = append(righe, r.AllegatoID.String()[34:]+"/"+r.Sezione+"/"+string(r.Esito)+"/"+r.Peso)
	}
	attese := "01/baseline/corretto/copertura 01/scenario/corretto/copertura 02/scenario/mancante/astensione 03//non_coperto/fuori_dal_gate"
	if strings.Join(righe, " ") != attese {
		t.Errorf("esiti %v\nattesi %s", righe, attese)
	}
	if out[2].Motivo != confronto.MotivoAttesoFileNonNelThread || out[3].Motivo != confronto.MotivoAttesoSenzaVoce {
		t.Errorf("motivi %q, %q", out[2].Motivo, out[3].Motivo)
	}
	permutati := confronto.ConfrontaConAtteso([]confronto.File{file[1], file[0]},
		confronto.Atteso{File: []confronto.FileAtteso{voci[2], voci[0], voci[1]}})
	if !reflect.DeepEqual(out, permutati) {
		t.Errorf("l'ordine degli ingressi cambia gli esiti:\n%+v\n%+v", out, permutati)
	}
}

// TestPesoNelGate (R30 b A): il peso di ogni esito; un esito fuori vocabolario non è mai «passato».
func TestPesoNelGate(t *testing.T) {
	attesi := map[confronto.EsitoAtteso]string{
		confronto.EsitoCorretto: "copertura", confronto.EsitoFuoriRichiesta: "copertura",
		confronto.EsitoAmbiguo: "astensione", confronto.EsitoMancante: "astensione",
		confronto.EsitoErrato: "bloccante", confronto.EsitoFalsaAssociazione: "bloccante",
		confronto.EsitoNonCoperto: "fuori_dal_gate", confronto.EsitoDaRivedere: "fuori_dal_gate",
		confronto.EsitoRiservato: "mai_passato", "inventato": "mai_passato",
	}
	for e, p := range attesi {
		if got := confronto.PesoNelGate(e); got != p {
			t.Errorf("PesoNelGate(%s) = %s, atteso %s", e, got, p)
		}
	}
}

// TestSezioniConNomiNeutri (P-10; R47 a, M-21): le sezioni hanno nomi neutri, senza clienti né sigle; nel JSON dell'atteso
// le chiavi sono snake_case.
func TestSezioniConNomiNeutri(t *testing.T) {
	sezioni := []string{confronto.SezioneScenario, confronto.SezioneBaseline, confronto.SezioneReali, confronto.SezioneDaRivedere}
	if strings.Join(sezioni, " ") != "scenario baseline reali da_rivedere" {
		t.Errorf("sezioni %v", sezioni)
	}
	b, err := json.Marshal(confronto.FileAtteso{AllegatoID: id(1), Sezione: confronto.SezioneDaRivedere, Riservato: "x",
		DipendeDa: "Q2", StatoAtteso: "candidato_scenario", Autorita: "scenario", BaseLetta: "7120100",
		Decisione: &confronto.Vecchio{Stato: "confermata"}, Invarianti: map[string]bool{"conta_nel_gate": false}})
	if err != nil {
		t.Fatal(err)
	}
	for _, chiave := range []string{`"allegato_id"`, `"sha256"`, `"atteso"`, `"target_base"`, `"sezione":"da_rivedere"`, `"riservato"`,
		`"dipende_da"`, `"stato_atteso"`, `"autorita"`, `"base_letta"`, `"decisione"`, `"invarianti"`} {
		if !strings.Contains(string(b), chiave) {
			t.Errorf("manca %s in %s", chiave, b)
		}
	}
}

// TestConfrontaProdotti (6.4.6 [agg.]; 6.4.9: originale, base, fase, quantità, evidenza in una cella): i prodotti attesi
// si legano ai candidati della mail per base, e fra due candidati con la stessa base al codice richiesto nominato; un
// campo non nominato non si controlla (R25 c-d); un atteso senza candidato è mancante, un candidato senza voce
// non_coperto; l'ordine non dipende da quello degli ingressi.
func TestConfrontaProdotti(t *testing.T) {
	cinque, tre := 5, 3
	vero, falso := true, false
	nuovi := []confronto.ProdottoNuovo{
		{CodiceRichiesto: "P7120100", Base: "7120100", Fase: "prototipo", Quantita: &cinque, QuantitaDaCella: true},
		{CodiceRichiesto: "P7120200", Base: "7120200", Fase: "prototipo", Quantita: &tre, QuantitaDaCella: true},
		{CodiceRichiesto: "7120300", Base: "7120300", Fase: "", Quantita: &cinque, QuantitaDaCella: false},
		{CodiceRichiesto: "P7120300", Base: "7120300", Fase: "prototipo", Quantita: &cinque, QuantitaDaCella: true},
		{CodiceRichiesto: "P7120900", Base: "7120900"},
	}
	attesi := []confronto.ProdottoAtteso{
		{CodiceRichiesto: "P7120100", Base: "7120100", Fase: "prototipo", Quantita: &cinque, QuantitaDaCella: &vero},
		{Base: "7120200", Fase: "prototipo", Quantita: &cinque, QuantitaDaCella: &falso},
		{CodiceRichiesto: "P7120300", Base: "7120300", Quantita: &cinque},
		{CodiceRichiesto: "P7120400", Base: "7120400"},
	}
	out := confronto.ConfrontaProdotti(nuovi, attesi)
	var righe []string
	for _, r := range out {
		codice := ""
		if r.Nuovo != nil {
			codice = r.Nuovo.CodiceRichiesto
		}
		righe = append(righe, r.Base+"/"+string(r.Esito)+"/"+codice+"/"+strings.Join(r.Differenze, ","))
	}
	attese := "7120100/corretto/P7120100/ 7120200/errato/P7120200/quantita,quantita_da_cella 7120300/corretto/P7120300/ " +
		"7120400/mancante// 7120300/non_coperto/7120300/ 7120900/non_coperto/P7120900/"
	if strings.Join(righe, " ") != attese {
		t.Errorf("esiti %v\nattesi %s", righe, attese)
	}
	permutati := confronto.ConfrontaProdotti([]confronto.ProdottoNuovo{nuovi[4], nuovi[3], nuovi[2], nuovi[1], nuovi[0]},
		[]confronto.ProdottoAtteso{attesi[3], attesi[1], attesi[2], attesi[0]})
	if !reflect.DeepEqual(out, permutati) {
		t.Errorf("l'ordine degli ingressi cambia gli esiti")
	}
}

// TestConfrontaProdottiLegameEsatto (R-47): un atteso senza codice non porta via il candidato che serve all'atteso con
// il codice: prima i legami esatti (base e codice), poi quelli per sola base. Tutti e due corretti.
func TestConfrontaProdottiLegameEsatto(t *testing.T) {
	nuovi := []confronto.ProdottoNuovo{{CodiceRichiesto: "P7120100", Base: "7120100"}, {CodiceRichiesto: "Q7120100", Base: "7120100"}}
	attesi := []confronto.ProdottoAtteso{{Base: "7120100"}, {CodiceRichiesto: "P7120100", Base: "7120100"}}
	var righe []string
	for _, r := range confronto.ConfrontaProdotti(nuovi, attesi) {
		codice, nominato := "", ""
		if r.Nuovo != nil {
			codice = r.Nuovo.CodiceRichiesto
		}
		if r.Atteso != nil {
			nominato = r.Atteso.CodiceRichiesto
		}
		righe = append(righe, nominato+"→"+codice+"/"+string(r.Esito))
	}
	if strings.Join(righe, " ") != "→Q7120100/corretto P7120100→P7120100/corretto" {
		t.Errorf("legami %v", righe)
	}
}

// TestConfrontaConLAtteso: con l'atteso Confronta riempie gli esiti per file e per prodotto, e l'impronta resta quella
// dell'anteprima, senza l'atteso (3.3.8: «la stessa nel banco e nell'anteprima»).
func TestConfrontaConLAtteso(t *testing.T) {
	file := []confronto.File{{AllegatoID: id(1), Vecchio: decisoSu(1, "7120101", "7120101"),
		Nuovo: valutato("figlio", "candidato_unico", []string{"7120101"}, candidato(1, "7120101"))}}
	prodotti := []confronto.ProdottoNuovo{{CodiceRichiesto: "P7120100", Base: "7120100"}}
	atteso := &confronto.Atteso{ThreadID: id(99), File: []confronto.FileAtteso{{AllegatoID: id(1), TargetBase: "7120101", Sezione: "baseline"}},
		Prodotti: []confronto.ProdottoAtteso{{Base: "7120100"}}}
	banco := confronto.Confronta(file, prodotti, atteso)
	anteprima := confronto.Confronta(file, prodotti, nil)
	if len(banco.ControAtteso) != 1 || banco.ControAtteso[0].Esito != confronto.EsitoCorretto ||
		len(banco.ProdottiControAtteso) != 1 || banco.ProdottiControAtteso[0].Esito != confronto.EsitoCorretto {
		t.Errorf("banco %+v / %+v", banco.ControAtteso, banco.ProdottiControAtteso)
	}
	if anteprima.ControAtteso != nil || anteprima.ProdottiControAtteso != nil {
		t.Errorf("l'anteprima ha esiti contro l'atteso")
	}
	if banco.Impronta == "" || banco.Impronta != anteprima.Impronta {
		t.Errorf("impronte %q e %q: devono coincidere", banco.Impronta, anteprima.Impronta)
	}
}
