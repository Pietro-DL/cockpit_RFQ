package bancoa

import (
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
)

// L1 — la traduzione fra attesi e motore (A1a-BA, A1b-24; R19 a-b, R25, R47 a; par.4.7.5 e 5.4.5 del piano A):
// i contesti (figlio_step → nodo_step, notazione puntata, nessun ripiego); la tabella delle chiavi, completa e
// con i soli nomi neutri, divisa fra A1a, A1b e decadute; ogni chiave di A1a sulle letture vere del motore, con
// il valore giusto e con uno sbagliato; da A1b.11 le chiavi del router e degli attributi su DaTesto più
// Interpreta con l'uso sconosciuto (le letture d'identità per funzione, le menzioni, la revisione in campo
// separato), anche loro con il valore giusto e con uno sbagliato, e la funzione del banco uguale a quella che
// Interpreta scrive nelle letture.
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

// TestChiaviDecaduteETipiNonPrevisti: riscritta per A1b.11. Le decadute restano rimandate; le letture d'identità
// sul corpo, che in A1a erano rimandate, si contano (con l'uso sconosciuto la lettura è una richiesta, R25 a);
// tipi non previsti, chiavi che si leggono con un'altra.
func TestChiaviDecaduteETipiNonPrevisti(t *testing.T) {
	e := valuta(t, "corpo", "PN 7654321", n("letture_identita", "0"), s("confronto_legacy", "x"))
	if e.Esito != CasoFallito {
		t.Fatalf("esito %q: %+v", e.Esito, e.Chiavi)
	}
	if k := chiave(e, "letture_identita"); k.Stato != ChiaveFallita || k.Ottenuto != "1" {
		t.Errorf("letture_identita sul corpo: %+v", k)
	}
	if k := chiave(e, "confronto_legacy"); k.Stato != ChiaveRimandata || k.Sessione != SessioneDecaduta {
		t.Errorf("chiave decaduta: %+v", k)
	}
	if e := valuta(t, "corpo", "PN 7654321", s("confronto_legacy", "x")); e.Esito != CasoRimandato {
		t.Errorf("solo una chiave decaduta: %q", e.Esito)
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

// chiaviDelRouter: le chiavi che il router e gli attributi di Interpreta rendono controllabili (A1b-24), nei nomi
// neutri di M-21 (R47 a). revisione_da_questo_campo, identita_da_famiglia_etichettata, stato, revisione e
// originale_conservato si controllavano già in A1a, sulle letture di forma: da A1b.11 leggono l'interpretazione.
var chiaviDelRouter = []string{
	"letture_identita", "base_menzionata", "identita_file_da_nota", "nessuna_fusione", "revisione_da_questo_campo",
	"fallback_generico_non_promuove", "identita_da_famiglia_etichettata", "stato", "cifre",
	"nessuna_inferenza_da_altro_profilo", "originale_conservato", "revisione",
}

// TestLeChiaviDelRouterSiLeggono (A1b-24): il decoder legge le chiavi del router dal YAML sintetico
// testdata/attesi_acme.yaml; ognuna è nella tabella con una regola di valutazione (nessuna resta rimandata) e
// con un nome neutro.
func TestLeChiaviDelRouterSiLeggono(t *testing.T) {
	presenti := map[string]bool{}
	for _, c := range attesiACME(t, nil).Casi {
		for _, k := range c.Atteso {
			presenti[k.Chiave] = true
		}
	}
	for _, k := range chiaviDelRouter {
		if !presenti[k] {
			t.Errorf("la chiave %q non è nel YAML sintetico", k)
		}
		if !ChiaveNota(k) || tabellaChiavi[k].valuta == nil {
			t.Errorf("la chiave %q non si valuta: %+v", k, tabellaChiavi[k])
		}
	}
	for _, k := range []string{"fallback_generico_non_promuove", "identita_file_da_nota", "base_menzionata", "nessuna_fusione",
		"cifre", "nessuna_inferenza_da_altro_profilo"} {
		if SessioneChiave(k) != SessioneA1b {
			t.Errorf("%s: sessione %q, attesa A1b", k, SessioneChiave(k))
		}
	}
	// Nessuna chiave degli attesi è rimandata per costruzione: senza regola di valutazione ci sono solo le decadute.
	for _, k := range ChiaviNote() {
		if tabellaChiavi[k].valuta == nil && SessioneChiave(k) != SessioneDecaduta {
			t.Errorf("la chiave %q non ha una regola di valutazione", k)
		}
	}
}

// TestChiaviA1bSulleLetture: le chiavi del router e degli attributi su DaTesto più Interpreta con l'uso
// sconosciuto (5.4.5), prima con i valori giusti (tutte passate), poi con uno sbagliato (quella chiave fallita).
func TestChiaviA1bSulleLetture(t *testing.T) {
	casi := []struct {
		nome, contesto, testo string
		chiavi                []ChiaveAttesa
	}{
		{"menzione nel testo del PDF (riga 16 del router; R25 a)", "testo_pdf", "vedi Q+700.099999.010 per il dettaglio", []ChiaveAttesa{
			n("letture_identita", "0"), s("base_menzionata", "Q+700.099999.010"), b("identita_file_da_nota", false),
			b("nessuna_fusione", true), b("fallback_generico_non_promuove", true),
		}},
		{"revisione in campo separato attribuita (5.4.6 punto 12; D9)", "cartiglio.revisione", "01", []ChiaveAttesa{
			s("revisione", "01"), s("revisione_da_questo_campo", "01"), n("cifre", "2"), s("stato", "attribuito"),
			b("originale_conservato", true), b("nessuna_inferenza_da_altro_profilo", true), n("letture_identita", "0"),
		}},
		{"revisione in campo separato non interpretabile (C-34, A-C09)", "cartiglio.revisione", "VEDI NOTA 00", []ChiaveAttesa{
			nullo("revisione"), s("stato", "non_interpretabile"), b("originale_conservato", true), n("letture_identita", "0"),
			b("nessuna_inferenza_da_altro_profilo", true),
		}},
		{"il generico non promuove (R25 e; A-C08)", "corpo", "rif. 7654321 senza etichetta", []ChiaveAttesa{
			b("identita_da_famiglia_etichettata", false), b("fallback_generico_non_promuove", true), n("letture_identita", "0"),
		}},
		{"richiesta dall'oggetto con l'uso sconosciuto (riga 2 del router)", "oggetto", "PN 7654321", []ChiaveAttesa{
			n("letture_identita", "1"), b("identita_da_famiglia_etichettata", true), b("fallback_generico_non_promuove", true),
			b("identita_file_da_nota", true), b("nessuna_fusione", true),
		}},
		{"identità del file dal codice del cartiglio (riga 8 del router)", "cartiglio.codice", "Q+700.099999.010", []ChiaveAttesa{
			n("letture_identita", "1"), s("base", "Q+700.099999.010"), b("originale_conservato", true), s("stato", "completa"),
		}},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			e := valuta(t, c.contesto, c.testo, c.chiavi...)
			if e.Esito != CasoPassato {
				t.Fatalf("esito %q: %+v; letture %+v; attributi %+v", e.Esito, e.Chiavi, e.Letture, e.Attributi)
			}
			for i, k := range c.chiavi {
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

// TestChiaviA1bSenzaEvidenze: una chiave che legge le menzioni o le revisioni fallisce dove non ce ne sono: mai un
// passato per vuoto.
func TestChiaviA1bSenzaEvidenze(t *testing.T) {
	e := valuta(t, "nome_file", "9123456A_2.pdf", s("base_menzionata", "9123456"))
	if k := chiave(e, "base_menzionata"); k.Stato != ChiaveFallita || k.Ottenuto != senzaMenzioni {
		t.Errorf("base menzionata su un nome del file: %+v", k)
	}
	e = valuta(t, "nome_file", "disegno.pdf", n("cifre", "2"))
	if k := chiave(e, "cifre"); k.Stato != ChiaveFallita || k.Ottenuto != senzaRevisioni {
		t.Errorf("cifre senza revisioni: %+v", k)
	}
	e = valuta(t, "nome_file", "disegno.pdf", b("originale_conservato", true), s("stato", "completa"), nullo("revisione"))
	for _, k := range e.Chiavi {
		if k.Stato != ChiaveFallita || k.Ottenuto != senzaLetture {
			t.Errorf("%s senza letture e senza attributi: %+v", k.Chiave, k)
		}
	}
	// Tre cifre attese: «01» è una revisione di due cifre.
	e = valuta(t, "cartiglio.revisione", "01", n("cifre", "3"))
	if k := chiave(e, "cifre"); k.Stato != ChiaveFallita || k.Ottenuto != "01: 2" {
		t.Errorf("cifre diverse: %+v", k)
	}
}

// TestPredicatiDelRouterSuScene: i predicati che sulle scene del YAML sintetico danno sempre lo stesso valore, su
// scene costruite in Go: una lettura unita a un'altra (deduplica della stessa occorrenza), una lettura di una
// famiglia che lo snapshot non ha, un'interpretazione di un altro cliente o con una revisione di un'altra famiglia.
func TestPredicatiDelRouterSuScene(t *testing.T) {
	m := insiemeACME(t).Motori[clienteACME]
	sel, _ := TraduciContesto("cartiglio.revisione")
	in, motivo := interpreta(m, sel, "01")
	if motivo != "" || len(in.Attributi) != 1 {
		t.Fatalf("%s %+v", motivo, in.Attributi)
	}
	if sc := nuovaScena(sel, "01", in, m, clienteACME); !soloIlProfilo(sc) {
		t.Fatalf("lo snapshot del profilo: %+v", in.Attributi)
	}
	altro := uuid.MustParse("00000000-0000-4000-8000-00000000ac02")
	if soloIlProfilo(nuovaScena(sel, "01", in, m, altro)) {
		t.Error("un'interpretazione di un altro cliente passa")
	}
	altraFamiglia := in
	altraFamiglia.Attributi = append([]motorea.AttributoLetto(nil), in.Attributi...)
	altraFamiglia.Attributi[0].ID = "a:" + altraFamiglia.Attributi[0].UnitaID + ":revisione:famiglia-inventata/rev"
	if soloIlProfilo(nuovaScena(sel, "01", altraFamiglia, m, clienteACME)) {
		t.Error("una revisione da una famiglia che lo snapshot non ha passa")
	}
	if f, r, ok := regolaDellAttributo(in.Attributi[0]); !ok || f != "acme-campo" || r != "rev-campo" {
		t.Errorf("regola dall'ID: %q %q %v", f, r, ok)
	}
	if _, _, ok := regolaDellAttributo(motorea.AttributoLetto{ID: "a:u:testo:revisione", UnitaID: "u:testo"}); ok {
		t.Error("un attributo senza regola ha una regola")
	}

	corpo, _ := TraduciContesto("corpo")
	in, motivo = interpreta(m, corpo, "PN 7654321")
	if motivo != "" || len(in.Letture) != 1 {
		t.Fatalf("%s %+v", motivo, in.Letture)
	}
	if !nessunaFusione(nuovaScena(corpo, "PN 7654321", in, m, clienteACME)) {
		t.Error("una lettura sola è una fusione")
	}
	unita := in
	unita.Letture = append([]motorea.LetturaCodice(nil), in.Letture...)
	unita.Letture[0].AltreUnita = []string{"u:altra"}
	if nessunaFusione(nuovaScena(corpo, "PN 7654321", unita, m, clienteACME)) {
		t.Error("una lettura con altre unità non è una fusione")
	}
	inventata := in
	inventata.Letture = append([]motorea.LetturaCodice(nil), in.Letture...)
	inventata.Letture[0].Forma.Famiglia = "famiglia-inventata"
	if nessunaLetturaSenzaFamiglia(nuovaScena(corpo, "PN 7654321", inventata, m, clienteACME)) {
		t.Error("una lettura d'identità senza una famiglia dello snapshot è passata (R25 e)")
	}
}

// TestLaFunzioneDelBancoEQuellaDiInterpreta: funzioneNelBanco, che la coerenza usa al posto di un'interpretazione,
// dà la funzione che Interpreta scrive nelle letture dei casi (una sola fonte della semantica: il router). Sui
// campi della revisione e sulla storia, dove il YAML sintetico non ha letture, vale la riga del router.
func TestLaFunzioneDelBancoEQuellaDiInterpreta(t *testing.T) {
	m := insiemeACME(t).Motori[clienteACME]
	for _, x := range []struct{ contesto, testo string }{
		{"nome_file", "9123456A_2.pdf"}, {"cartiglio.codice", "9123456A2"}, {"radice_step.id", "9123456A"},
		{"figlio_step.id", "9123456A"}, {"corpo", "PN 7654321"}, {"oggetto", "PN 7654321"},
		{"testo_pdf", "vedi Q+700.099999.010 per il dettaglio"},
	} {
		sel, err := TraduciContesto(x.contesto)
		if err != nil {
			t.Fatal(err)
		}
		in, motivo := interpreta(m, sel, x.testo)
		if motivo != "" || len(in.Letture) == 0 {
			t.Fatalf("%s: %s %+v", x.contesto, motivo, in.Letture)
		}
		for _, l := range in.Letture {
			if l.Funzione != funzioneNelBanco(sel) {
				t.Errorf("%s: Interpreta %q, banco %q", x.contesto, l.Funzione, funzioneNelBanco(sel))
			}
		}
	}
	for contesto, f := range map[string]motorea.Funzione{
		"cartiglio.revisione": motorea.FunzAttributo, "storia": motorea.FunzMenzione, "nome_file": motorea.FunzIdentitaFile,
		"figlio_step.id": motorea.FunzStruttura, "corpo": motorea.FunzRichiesta,
	} {
		sel, _ := TraduciContesto(contesto)
		if got := funzioneNelBanco(sel); got != f {
			t.Errorf("%s: %q, attesa %q", contesto, got, f)
		}
	}
}
