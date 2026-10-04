package bancoa

import (
	"sort"
	"strings"
	"testing"
)

// L1 — la traduzione fra attesi e motore (A1a-BA; R19 a-b, R25, R47 a; par.4.7.5 del piano A): i contesti
// (figlio_step → nodo_step, notazione puntata, nessun ripiego); la tabella delle chiavi, completa e con i soli
// nomi neutri, divisa fra A1a, A1b e decadute; ogni chiave di A1a sulle letture vere del motore, con il
// valore giusto e con uno sbagliato.
//
// I clienti di questi test sono inventati. Non è pigrizia: gli attesi veri contengono codici e nomi dei
// clienti, e questo repository è pubblico. Il cliente è ACME (testdata/regole/acme.v1.json); i testi sono
// inventati con la forma dei codici del piano.

func TestTraduciContesto(t *testing.T) {
	buoni := map[string]string{
		"figlio_step.id":      "nodo_step.id",
		"figlio_step.nome":    "nodo_step.nome",
		"radice_step.id":      "radice_step.id",
		"nome_file":           "nome_file",
		"cartiglio.codice":    "cartiglio.codice",
		"cartiglio.revisione": "cartiglio.revisione",
		"testo_pdf":           "testo_pdf",
		"corpo":               "corpo",
		"oggetto":             "oggetto",
	}
	for in, atteso := range buoni {
		s, err := TraduciContesto(in)
		if err != nil || s.String() != atteso {
			t.Errorf("%q → %q, %v; atteso %q", in, s.String(), err, atteso)
		}
	}
	for _, cattivo := range []string{"figlio_step", "figlio_step.codice", "cartiglio.*", "nome_file.codice", "pagina", "Corpo", " corpo", ""} {
		if _, err := TraduciContesto(cattivo); err == nil {
			t.Errorf("%q accettato: nessun ripiego (R19 b)", cattivo)
		}
	}
}

// chiaviDegliAttesi: le chiavi di atteso che il file degli attesi usa, nei nomi neutri (M-21). Ognuna deve
// essere nota: una chiave fuori tabella rende gli attesi illeggibili.
var chiaviDegliAttesi = []string{
	"affisso", "base", "base_candidata", "base_menzionata", "basi", "categorie", "cifre", "codice_richiesto",
	"completa", "completamento_inventato", "debole", "decorazione_nome_file", "destinazione",
	"equivalenza_slash_attiva", "etichetta", "fallback_generico_non_promuove", "fase", "forma_distinta", "forte",
	"identita_da_famiglia_etichettata", "identita_file_da_nota", "identita_include_stato_pdm",
	"identita_include_token", "involucro", "letture_identita", "marcatore", "match_forma_lavagna",
	"match_forma_osservata", "nessuna_fusione", "nessuna_inferenza_da_altro_profilo", "non_equivalente_a",
	"numero_codici", "originale_conservato", "plus_conservato", "revisione", "revisione_da_questo_campo",
	"revisione_dal_nome", "revisione_dal_token", "revisione_numerica", "richiede_verifica",
	"ripetizioni_concordanti", "secondo_codice_non_perso", "segmenti_mancanti", "semantica_S", "stato",
	"stato_pdm", "target_unico", "token_conservato", "token_revisione", "tripla_finale_parte_base",
}

func TestTabellaDelleChiavi(t *testing.T) {
	for _, k := range chiaviDegliAttesi {
		if !ChiaveNota(k) {
			t.Errorf("la chiave %q degli attesi non è nella tabella", k)
		}
	}
	a1b := []string{"fallback_generico_non_promuove", "identita_file_da_nota", "base_menzionata", "nessuna_fusione",
		"cifre", "nessuna_inferenza_da_altro_profilo"}
	for _, k := range a1b {
		if SessioneChiave(k) != SessioneA1b {
			t.Errorf("%s: sessione %q, attesa A1b", k, SessioneChiave(k))
		}
	}
	for _, k := range chiaviDegliAttesi {
		if !dentro(k, a1b) && SessioneChiave(k) != SessioneA1a {
			t.Errorf("%s: sessione %q, attesa A1a", k, SessioneChiave(k))
		}
	}
	for _, k := range []string{"letture_identita_famiglia", "diagnostica_codice_non_riconosciuto", "confronto_legacy"} {
		if SessioneChiave(k) != SessioneDecaduta {
			t.Errorf("%s: sessione %q, attesa decaduta (M-01)", k, SessioneChiave(k))
		}
	}
	for _, k := range []string{"chiave_inventata", "Base", "base ", "atteso_conservativo"} {
		if ChiaveNota(k) {
			t.Errorf("%q è nota: una chiave sconosciuta va rifiutata", k)
		}
	}
	tutte := ChiaviNote()
	if !sort.StringsAreSorted(tutte) {
		t.Error("ChiaviNote non è in ordine")
	}
	// I nomi della tabella sono neutri: minuscole, cifre e «_», salvo la S di semantica_S (una lettera, il
	// nome dell'affisso nel contratto).
	for _, k := range tutte {
		for _, r := range strings.ReplaceAll(k, "semantica_S", "semantica_s") {
			if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_') {
				t.Errorf("chiave %q: carattere %q", k, r)
			}
		}
	}
}

// valuta: un caso costruito in Go sul motore ACME, e l'esito di una chiave.
func valuta(t *testing.T, contesto, testo string, chiavi ...ChiaveAttesa) EsitoCaso {
	t.Helper()
	a := Attesi{Casi: []CasoContratto{{ID: "caso-acme-x", Profilo: "acme", StatoAtteso: StatoAttesoDefinito,
		Contesto: contesto, Testo: testo, Atteso: chiavi}}}
	return EseguiCasiContratto(a, insiemeACME(t), profiliACME)[0]
}

func s(chiave, v string) ChiaveAttesa {
	return ChiaveAttesa{Chiave: chiave, Valore: ValoreAtteso{Tipo: TipoStringa, Testo: v}}
}

func n(chiave, v string) ChiaveAttesa {
	return ChiaveAttesa{Chiave: chiave, Valore: ValoreAtteso{Tipo: TipoIntero, Testo: v}}
}

func b(chiave string, v bool) ChiaveAttesa {
	testo := "false"
	if v {
		testo = "true"
	}
	return ChiaveAttesa{Chiave: chiave, Valore: ValoreAtteso{Tipo: TipoBooleano, Testo: testo}}
}

func nullo(chiave string) ChiaveAttesa {
	return ChiaveAttesa{Chiave: chiave, Valore: ValoreAtteso{Tipo: TipoNullo}}
}

func lista(chiave string, el ...string) ChiaveAttesa {
	return ChiaveAttesa{Chiave: chiave, Valore: ValoreAtteso{Tipo: TipoLista, Elementi: el}}
}

// TestChiaviA1aSulleLetture: ogni gruppo di chiavi, prima con i valori giusti (tutte passate), poi con uno
// sbagliato (quella chiave fallita, con l'ottenuto).
func TestChiaviA1aSulleLetture(t *testing.T) {
	casi := []struct {
		nome, contesto, testo string
		chiavi                []ChiaveAttesa
	}{
		{"prefisso, involucro, stato PDM e token (D2, Q1)", "nome_file", "ACME-030P7120100 00 IN_WORK.stp", []ChiaveAttesa{
			s("base", "7120100"), s("affisso", "P"), s("fase", "prototipo"), nullo("destinazione"),
			s("codice_richiesto", "P7120100"), s("involucro", "ACME-030"), s("stato_pdm", "IN_WORK"),
			s("token_conservato", "00"), b("identita_include_stato_pdm", false), b("identita_include_token", false),
			b("originale_conservato", true), nullo("revisione_dal_nome"), b("completa", true), b("target_unico", true),
			s("stato", "completa"), s("semantica_S", "attribuita"),
		}},
		{"affisso riconosciuto e non attribuito (D5)", "oggetto", "S9.123.4567.3", []ChiaveAttesa{
			s("affisso", "S"), s("semantica_S", "non_attribuita"), nullo("fase"), s("base", "9.123.4567.3"),
			b("tripla_finale_parte_base", true),
		}},
		{"forma parziale (A-C07)", "corpo", "9.123.4567", []ChiaveAttesa{
			lista("segmenti_mancanti", "T"), b("completa", false), b("completamento_inventato", false),
			s("stato", "parziale"), s("base", "9.123.4567"),
		}},
		{"revisione forte e debole (D4)", "corpo", "9.123.4567.3/01", []ChiaveAttesa{
			s("revisione", "01"), n("forte", "0"), n("debole", "1"), b("equivalenza_slash_attiva", false),
			b("richiede_verifica", false), s("non_equivalente_a", "1"),
		}},
		{"token di revisione sospeso (D5)", "corpo", "9.123.4567.3/xx", []ChiaveAttesa{
			s("token_revisione", "xx"), nullo("revisione_numerica"), nullo("revisione"), b("richiede_verifica", true),
			s("base_candidata", "9.123.4567.3"), s("stato", "da_verificare"), s("non_equivalente_a", "xx"),
		}},
		{"due basi in un nome (A1a-CNF)", "nome_file", "9123456_9123457.zip", []ChiaveAttesa{
			lista("basi", "9123457", "9123456"), n("numero_codici", "2"), b("secondo_codice_non_perso", true),
		}},
		{"etichetta obbligatoria (A-C08)", "corpo", "PN 7654321", []ChiaveAttesa{
			s("etichetta", "PN"), s("base", "7654321"), b("identita_da_famiglia_etichettata", true),
		}},
		{"marcatore sul nodo (D1, R19 a)", "figlio_step.id", "9123456A", []ChiaveAttesa{
			s("marcatore", "A"), nullo("revisione_da_questo_campo"), nullo("revisione_dal_token"),
		}},
		{"nessuna lettura sul nome (letture d'identità zero)", "nome_file", "disegno.pdf", []ChiaveAttesa{
			n("letture_identita", "0"), b("match_forma_osservata", false), b("identita_da_famiglia_etichettata", false),
			n("numero_codici", "0"),
		}},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			e := valuta(t, c.contesto, c.testo, c.chiavi...)
			if e.Esito != CasoPassato {
				t.Fatalf("esito %q: %+v; letture %+v", e.Esito, e.Chiavi, e.Letture)
			}
			// Ogni chiave, una alla volta, con un valore sbagliato: fallisce.
			for i, k := range c.chiavi {
				if k.Chiave == "letture_identita" {
					continue // diverso da zero è A1b: lo prova TestChiaviCheNonSiDecidonoInA1a
				}
				sbagliata := append([]ChiaveAttesa(nil), c.chiavi...)
				sbagliata[i] = valoreSbagliato(k)
				e := valuta(t, c.contesto, c.testo, sbagliata...)
				if got := chiave(e, k.Chiave); got.Stato != ChiaveFallita || e.Esito != CasoFallito {
					t.Errorf("%s con %s: %+v (esito %s)", k.Chiave, sbagliata[i].Valore.String(), got, e.Esito)
				}
			}
		})
	}
}

// valoreSbagliato: lo stesso tipo con un altro valore, o null al posto di un valore e viceversa.
func valoreSbagliato(k ChiaveAttesa) ChiaveAttesa {
	v := k.Valore
	switch v.Tipo {
	case TipoBooleano:
		if v.Testo == "true" {
			v.Testo = "false"
		} else {
			v.Testo = "true"
		}
	case TipoNullo:
		v = ValoreAtteso{Tipo: TipoStringa, Testo: "99"}
	case TipoIntero:
		v.Testo = "7"
	case TipoLista:
		v.Elementi = append(append([]string(nil), v.Elementi...), "inventato")
	default:
		if k.Chiave != "non_equivalente_a" {
			v.Testo += "-inventato"
			break
		}
		// Fallisce solo con il valore della revisione letta; con un token sospeso nessun valore è equivalente,
		// e la chiave fallisce solo con un tipo che non si confronta.
		if v.Testo != "1" {
			return ChiaveAttesa{Chiave: k.Chiave, Valore: ValoreAtteso{Tipo: TipoMappa}}
		}
		v.Testo = "01"
	}
	return ChiaveAttesa{Chiave: k.Chiave, Valore: v}
}

// TestChiaviCheNonSiDecidonoInA1a: rimandate, decadute, letture d'identità fuori dai selettori d'identità,
// tipi non previsti, chiavi che si leggono con un'altra.
func TestChiaviCheNonSiDecidonoInA1a(t *testing.T) {
	e := valuta(t, "corpo", "PN 7654321", n("letture_identita", "0"), s("confronto_legacy", "x"))
	if e.Esito != CasoRimandato {
		t.Fatalf("esito %q: %+v", e.Esito, e.Chiavi)
	}
	if k := chiave(e, "letture_identita"); k.Stato != ChiaveRimandata {
		t.Errorf("letture_identita sul corpo: %+v", k)
	}
	if k := chiave(e, "confronto_legacy"); k.Stato != ChiaveRimandata || k.Sessione != SessioneDecaduta {
		t.Errorf("chiave decaduta: %+v", k)
	}

	e = valuta(t, "corpo", "9.123.4567.3/01", s("forte", "zero"))
	if k := chiave(e, "forte"); k.Stato != ChiaveFallita || k.Motivo != tipoNonPrevisto {
		t.Errorf("tipo non previsto: %+v", k)
	}
	e = valuta(t, "nome_file", "9123456_9123457.zip", b("secondo_codice_non_perso", true))
	if k := chiave(e, "secondo_codice_non_perso"); k.Stato != ChiaveFallita || !strings.Contains(k.Motivo, "basi") {
		t.Errorf("secondo codice senza basi: %+v", k)
	}
	e = valuta(t, "nome_file", "disegno.pdf", s("base", "9123456"))
	if k := chiave(e, "base"); k.Stato != ChiaveFallita || k.Ottenuto != senzaLetture {
		t.Errorf("base senza letture: %+v", k)
	}
}

// TestListaAttesaVuota: una lista vuota negli attesi vuol dire «nessuno», non «qualunque cosa»: passa solo se
// la lettura non ha valori di quella chiave (nessuna chiave passa per vuoto).
func TestListaAttesaVuota(t *testing.T) {
	casi := []struct {
		nome, contesto, testo string
		chiave                ChiaveAttesa
		stato                 string
	}{
		{"nessun segmento mancante su una completa", "corpo", "9.123.4567.3", lista("segmenti_mancanti"), ChiavePassata},
		{"nessun segmento mancante su una parziale (A-C07)", "corpo", "9.123.4567", lista("segmenti_mancanti"), ChiaveFallita},
		{"nessun involucro dove non c'è", "corpo", "9.123.4567.3", lista("involucro"), ChiavePassata},
		{"nessun involucro dove c'è", "nome_file", "ACME-030P7120100 00 IN_WORK.stp", lista("involucro"), ChiaveFallita},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			if k := chiave(valuta(t, c.contesto, c.testo, c.chiave), c.chiave.Chiave); k.Stato != c.stato {
				t.Fatalf("%+v, atteso %s", k, c.stato)
			}
		})
	}
}

// TestFormaDistinta: la lettura viene da una forma che non legge gli altri nomi dello stesso profilo.
func TestFormaDistinta(t *testing.T) {
	a := Attesi{Casi: []CasoContratto{
		{ID: "caso-acme-a", Profilo: "acme", StatoAtteso: StatoAttesoDefinito, Contesto: "nome_file", Testo: "9123456A_2.pdf",
			Atteso: []ChiaveAttesa{b("forma_distinta", true)}},
		{ID: "caso-acme-b", Profilo: "acme", StatoAtteso: StatoAttesoDefinito, Contesto: "nome_file", Testo: "9123457_9123458.zip",
			Atteso: []ChiaveAttesa{s("base", "9123457")}},
		{ID: "caso-acme-c", Profilo: "acme", StatoAtteso: StatoAttesoDefinito, Contesto: "nome_file", Testo: "9123459A_3.pdf",
			Atteso: []ChiaveAttesa{b("forma_distinta", false)}},
	}}
	es := perID(EseguiCasiContratto(a, insiemeACME(t), profiliACME))
	if k := chiave(es["caso-acme-a"], "forma_distinta"); k.Stato != ChiavePassata {
		t.Errorf("forma diversa da quella di un altro nome: %+v", k)
	}
	// Il terzo caso è letto dalla stessa forma del primo, ma il secondo è letto da un'altra: anche lui è distinto,
	// quindi l'atteso «false» non passa.
	if k := chiave(es["caso-acme-c"], "forma_distinta"); k.Stato != ChiaveFallita {
		t.Errorf("forma distinta attesa falsa: %+v", k)
	}
	if es["caso-acme-b"].Esito != CasoFallito {
		t.Errorf("basi diverse dall'atteso: %+v", es["caso-acme-b"])
	}
}
