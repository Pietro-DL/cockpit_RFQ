package classificazione

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// L1 — Smistamento M3 (A5.16.6): la fotografia di una decisione sulla posta. Il rango della RFQ scelta
// nell'ordine del pannello, e il JSON del motivo, stabile e rileggibile. RFQ finte (T1, T2, T3).

// scenaTreRFQ: T1 con R0 verificato (molto forte), T2 con R3 del buyer e l'oggetto (medio-forte, due
// tipi), T3 con il solo ConversationID (debole). L'ordine del pannello è T1, T2, T3.
func scenaTreRFQ() []CandidatoRFQ {
	return RaggruppaEOrdina([]Candidato{
		NuovoCandidato("T3", TipoR1Solo, "stessa conversazione", false),
		NuovoCandidato("T2", TipoR2Oggetto, "stesso oggetto", false),
		NuovoCandidato("T2", TipoR3CodiceBuyer, "codice 7120001 dello stesso buyer", false),
		NuovoCandidato("T1", TipoR0InReplyTo, "risponde a una mail della RFQ", false),
	})
}

// 248 — il rango della RFQ scelta: la prima card = 1, la seconda = 2; «Altra RFQ…» fuori dall'elenco = 0
// con fuori_lista; una RFQ nuova con candidati = 0 senza fuori_lista; «Ignora» senza scelto; il pari
// merito si registra; senza candidati niente primo.
func TestIlRangoDelCandidatoScelto(t *testing.T) {
	c := scenaTreRFQ()
	if len(c) != 3 || c[0].ThreadID != "T1" || c[1].ThreadID != "T2" || c[2].ThreadID != "T3" {
		t.Fatalf("scena: %+v", c)
	}
	primo := &PrimoRFQ{Thread: "T1", Livello: "molto_forte", Score: 98, Tipi: []string{TipoR0InReplyTo}}

	s := SceltaDa(c, GestoAggancia, "T1", EventoArrivoCAD, AttoDocumentiAggiuntivi)
	if s.Scelto == nil || s.Scelto.Rango != 1 || s.Scelto.Livello != "molto_forte" || s.Scelto.Score != 98 || s.FuoriLista {
		t.Errorf("la prima card: %+v", s.Scelto)
	}
	s = SceltaDa(c, GestoAggancia, "T2", EventoArrivoCAD, AttoDocumentiAggiuntivi)
	if want := (&SceltoRFQ{Rango: 2, Livello: "medio_forte", Score: 72, Tipi: []string{TipoR3CodiceBuyer, TipoR2Oggetto}}); !reflect.DeepEqual(s.Scelto, want) {
		t.Errorf("la seconda card: %+v, attesa %+v", s.Scelto, want)
	}
	if !reflect.DeepEqual(s.Primo, primo) || s.Su != 3 || s.PariMerito || s.FuoriLista {
		t.Errorf("con la seconda: primo %+v, su %d, pari merito %v, fuori lista %v", s.Primo, s.Su, s.PariMerito, s.FuoriLista)
	}
	if s.Gesto != GestoAggancia || s.Evento != EventoArrivoCAD || s.Atto != AttoDocumentiAggiuntivi || s.Regole != VersioneRegoleAggancio || s.V != 1 {
		t.Errorf("gesto, evento, atto, regole, versione: %+v", s)
	}
	if s.Frase != "decisione dell'operatore" {
		t.Errorf("la frase di sempre deve restare dentro: %q", s.Frase)
	}
	s = SceltaDa(c, GestoAggancia, "T3", "", "")
	if s.Scelto.Rango != 3 || s.Scelto.Livello != "debole" || s.Scelto.Score != 40 {
		t.Errorf("la terza card: %+v", s.Scelto)
	}

	// «Altra RFQ…»: una RFQ che non era fra i candidati
	s = SceltaDa(c, GestoAggancia, "T9", "", "")
	if want := (&SceltoRFQ{Rango: 0, Livello: "nessuno", Score: 0, Tipi: []string{}}); !reflect.DeepEqual(s.Scelto, want) || !s.FuoriLista {
		t.Errorf("fuori lista: %+v, fuori_lista %v", s.Scelto, s.FuoriLista)
	}
	// «Crea RFQ» con candidati: nessuno era quello giusto, ma non è una scelta «fuori lista»
	s = SceltaDa(c, GestoNuovaRFQ, "TNUOVA", "", "")
	if s.Scelto == nil || s.Scelto.Rango != 0 || s.FuoriLista || s.Su != 3 || !reflect.DeepEqual(s.Primo, primo) {
		t.Errorf("RFQ nuova con candidati: %+v", s)
	}
	if s.Frase != "decisione dell'operatore" {
		t.Errorf("RFQ nuova: la frase %q", s.Frase)
	}
	// «Ignora»: nessuna scelta, il primo resta
	s = SceltaDa(c, GestoIgnora, "", "", "")
	if s.Scelto != nil || s.FuoriLista || !reflect.DeepEqual(s.Primo, primo) || s.Frase != "chiuso senza RFQ" {
		t.Errorf("ignora: %+v", s)
	}
	// senza candidati: niente primo, su 0; la RFQ nuova ha rango 0
	s = SceltaDa(nil, GestoNuovaRFQ, "TNUOVA", EventoNuovaRFQ, AttoRichiestaOfferta)
	if s.Primo != nil || s.Su != 0 || s.Scelto == nil || s.Scelto.Rango != 0 || s.PariMerito {
		t.Errorf("RFQ nuova senza candidati: %+v", s)
	}

	// pari merito (P37): due RFQ con R3 dello stesso buyer. Si registra, e il rango resta la posizione
	// della card (le card hanno comunque un ordine, anche se nessuna è proposta)
	pari := RaggruppaEOrdina([]Candidato{
		NuovoCandidato("TA", TipoR3CodiceBuyer, "codice 7120001", false),
		NuovoCandidato("TB", TipoR3CodiceBuyer, "codice 7120001", false),
	})
	s = SceltaDa(pari, GestoAggancia, "TB", "", "")
	if !s.PariMerito || s.Scelto.Rango != 2 || s.Su != 2 {
		t.Errorf("pari merito: %+v, scelto %+v", s, s.Scelto)
	}

	// il marcatore della mail preparata dal Cockpit è una card come le altre, con il suo tipo
	m := RaggruppaEOrdina([]Candidato{
		NuovoCandidato("T2", TipoR2Oggetto, "stesso oggetto", false),
		NuovoCandidato("T1", TipoMarcatore, "è la nostra mail preparata dal Cockpit", false),
	})
	s = SceltaDa(m, GestoAggancia, "T1", "", "")
	if s.Scelto.Rango != 1 || !reflect.DeepEqual(s.Scelto.Tipi, []string{TipoMarcatore}) || s.Su != 2 {
		t.Errorf("la card del marcatore: %+v", s.Scelto)
	}

	// il giro degli orfani: il candidato proposto, con il suo rango fra quelli dell'orfano
	s = SceltaDa(c, GestoCandidato, "T3", "", "")
	if s.Gesto != GestoCandidato || s.Scelto.Rango != 3 || s.FuoriLista {
		t.Errorf("candidato: %+v", s)
	}
	for _, g := range GestiMisurati {
		if g == GestoCandidato || g == GestoMarcatore {
			t.Errorf("il gesto %q è automatico e non si misura", g)
		}
	}
}

// 249 — il motivo è un JSON stabile: campi e ordine fissi, la versione e le regole presenti, e riletto
// torna identico. Una riga di prima (una frase) non si rilegge come fotografia.
func TestIlMotivoDiCalibrazioneEUnJSONStabile(t *testing.T) {
	s := SceltaDa(scenaTreRFQ(), GestoAggancia, "T2", EventoRevisioneCAD, AttoRevisioneDocumenti)
	want := `{"v":1,"frase":"decisione dell'operatore","gesto":"aggancia",` +
		`"scelto":{"rango":2,"livello":"medio_forte","score":72,"tipi":["R3CodiceBuyer","R2Oggetto"]},` +
		`"primo":{"thread":"T1","livello":"molto_forte","score":98,"tipi":["R0InReplyTo"]},` +
		`"su":3,"pari_merito":false,"fuori_lista":false,` +
		`"evento":"REVISIONE_CAD","atto":"revisione_documenti","regole":"aggancio-2"}`
	if got := s.Motivo(); got != want {
		t.Errorf("motivo:\n%s\natteso:\n%s", got, want)
	}
	// «ignora» e «senza candidati»: i campi ci sono tutti, null dove non c'è niente
	vuota := SceltaDa(nil, GestoIgnora, "", EventoAltro, AttoIncerto).Motivo()
	if want := `{"v":1,"frase":"chiuso senza RFQ","gesto":"ignora","scelto":null,"primo":null,"su":0,"pari_merito":false,"fuori_lista":false,"evento":"ALTRO","atto":"incerto","regole":"aggancio-2"}`; vuota != want {
		t.Errorf("ignora senza candidati:\n%s\natteso:\n%s", vuota, want)
	}
	// ritorno identico, per ogni gesto
	for _, x := range []Scelta{s, SceltaDa(scenaTreRFQ(), GestoNuovaRFQ, "N", "", ""), SceltaDa(scenaTreRFQ(), GestoIgnora, "", "", ""),
		SceltaDa(scenaTreRFQ(), GestoAggancia, "T9", "", "")} {
		letta, ok := LeggiScelta(x.Motivo())
		if !ok || !reflect.DeepEqual(letta, x) {
			t.Errorf("riletta %+v (%v), attesa %+v", letta, ok, x)
		}
		var generico map[string]any
		if err := json.Unmarshal([]byte(x.Motivo()), &generico); err != nil {
			t.Errorf("non è un JSON valido: %v", err)
		}
		for _, k := range []string{"v", "frase", "gesto", "scelto", "primo", "su", "pari_merito", "fuori_lista", "evento", "atto", "regole"} {
			if _, ok := generico[k]; !ok {
				t.Errorf("gesto %s: manca il campo %q", x.Gesto, k)
			}
		}
	}
	// la frase del giro degli orfani passa com'è: né «·» né «<» diventano · o <
	c := SceltaDa(scenaTreRFQ(), GestoCandidato, "T3", "", "")
	c.Frase = "orfano della stessa conversazione: candidato debole · score 40 <prova>"
	if m := c.Motivo(); !strings.Contains(m, "debole · score 40 <prova>") || strings.Contains(m, "\n") {
		t.Errorf("la frase dentro il JSON: %s", m)
	}
	// le righe di prima sono frasi, e non si leggono come fotografie
	for _, vecchia := range []string{"decisione dell'operatore", "chiuso senza RFQ", "", `{"frase":"senza versione"}`, "{rotto"} {
		if _, ok := LeggiScelta(vecchia); ok {
			t.Errorf("%q letta come fotografia", vecchia)
		}
	}
	// nessuno score è una percentuale
	if strings.Contains(s.Motivo(), "%") {
		t.Error("il motivo contiene «%»")
	}
}

func TestLeFasceDegliScoreDellaPosta(t *testing.T) {
	for score, fascia := range map[int]string{98: "90-100", 90: "90-100", 88: "70-89", 72: "70-89", 62: "50-69", 45: "30-49", 40: "30-49", 20: "0-29", 0: "0-29"} {
		if got := FasciaScore(score); got != fascia {
			t.Errorf("FasciaScore(%d) = %s, attesa %s", score, got, fascia)
		}
	}
}
