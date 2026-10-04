package grammatica

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"promatec/cockpit/internal/core/estrazione/evidenze"
)

// L1 — la validazione delle grammatiche v1 (A-C01 pubblica; piano A, par.4.4.4, regole 1-15; parte 1 §7.1,
// §7.5, §12; C-10, C-12, R6, R17 c): ogni violazione del contratto dà un errore esplicito con il suo codice
// e il percorso del campo, mai un ripiego, nemmeno verso il formato legacy.
//
// I clienti di questi test sono inventati. Non è pigrizia: le famiglie di codice dei clienti veri sono un
// dato dell'azienda e questo repository è pubblico, e una prova che dipendesse da esse diventerebbe rossa il
// giorno in cui un cliente cambia convenzione. La grammatica è di ACME, con i nomi neutri delle famiglie
// (acme-prefisso, acme-punti, acme-etichetta, acme-documento); basi, letterali ed esempi sono inventati e
// attivano un meccanismo, mai il significato di un cliente (R20 a).

// grammaticaACME: una grammatica valida che usa ogni pezzo del contratto attivo in A1: affisso con fase,
// involucro, forma parziale in coda, revisione inline con forte, debole e token sospeso, etichetta
// obbligatoria, token con rimando, suffisso con ripetizione della base, stato PDM, revisione in campo
// separato, una riserva. Ogni prova ne cambia un pezzo solo.
const grammaticaACME = `{
  "versione_schema": 1,
  "cliente": {"id": "00000000-0000-4000-8000-00000000ac01", "ragione_sociale": "ACME S.p.A."},
  "profilo": {"stato": "parziale", "riserve": [{"id": "Q-ACME-1", "motivo": "domanda aperta inventata", "regole": ["xx"]}]},
  "famiglie_codice": [
    {
      "id": "acme-prefisso",
      "namespace": "acme",
      "descrizione": "codici inventati con il prefisso di fase",
      "ruoli": ["prodotto", "componente"],
      "base": {"segmenti": [{"nome": "numero", "pattern": "5[0-9]{6}", "identitario": true}],
               "maiuscole": "esatte", "normalizza": "nessuna", "confine_prima": "alnum_ascii", "confine_dopo": "alnum_ascii"},
      "forme": [
        {"id": "mail", "selettori": ["corpo", "storia"], "stato": "attiva", "completa": true,
         "parti": [{"tipo": "affisso", "rif": "fase", "min": 0, "max": 1}, {"tipo": "base", "min": 1, "max": 1}]},
        {"id": "nome", "selettori": ["nome_file"], "stato": "attiva", "completa": true,
         "parti": [{"tipo": "decorazione", "rif": "pacchetto", "min": 0, "max": 1},
                   {"tipo": "affisso", "rif": "fase", "min": 0, "max": 1},
                   {"tipo": "base", "min": 1, "max": 1}]},
        {"id": "cartiglio", "selettori": ["cartiglio.codice"], "stato": "attiva", "completa": true,
         "parti": [{"tipo": "base", "min": 1, "max": 1}]}
      ],
      "affissi": [{"id": "fase", "letterali": ["P"], "posizione": "prefisso",
                   "riconoscimento": ["corpo", "storia", "nome_file"], "attribuzione": ["corpo", "storia", "nome_file"],
                   "valore": {"fase": "prototipo"}}],
      "decorazioni": [{"id": "pacchetto", "tipo": "involucro", "sottotipo": "riferimento_pacchetto",
                       "letterali": ["ACME-030"], "selettori": ["nome_file"]}],
      "esempi": [
        {"id": "e1", "origine": "sintetico", "selettore": "corpo", "testo": "P5123456",
         "atteso": {"letture": [{"forma": "mail", "base": "5123456", "affissi": ["fase"]}]}},
        {"id": "e2", "origine": "sintetico", "rif_caso": "ACME-CASO-1", "selettore": "nome_file", "testo": "ACME-030P5123456.pdf",
         "atteso": {"letture": [{"forma": "nome", "base": "5123456", "decorazioni": ["pacchetto"]}]}},
        {"id": "e3", "origine": "sintetico", "selettore": "cartiglio.codice", "testo": "5123456",
         "atteso": {"letture": [{"forma": "cartiglio"}]}},
        {"id": "e4", "origine": "sintetico", "selettore": "corpo", "testo": "ACME-030P5123456", "atteso": {"nessuna": true}}
      ]
    },
    {
      "id": "acme-punti",
      "namespace": "acme",
      "ruoli": ["componente"],
      "categorie": ["minuteria"],
      "base": {"segmenti": [{"nome": "a", "letterale": "8", "identitario": true},
                            {"nome": "b", "pattern": "[0-9]{3}", "separatore": ".", "identitario": true},
                            {"nome": "T", "pattern": "[0-9]", "separatore": ".", "identitario": true}],
               "maiuscole": "esatte", "normalizza": "nessuna", "confine_prima": "parola_ascii", "confine_dopo": "parola_ascii"},
      "forme": [
        {"id": "completa", "selettori": ["corpo"], "stato": "attiva", "completa": true,
         "parti": [{"tipo": "base", "min": 1, "max": 1}, {"tipo": "revisione", "rif": "rev", "min": 0, "max": 1}]},
        {"id": "parziale", "selettori": ["corpo"], "stato": "attiva", "completa": false, "segmenti_mancanti": ["T"],
         "parti": [{"tipo": "base", "min": 1, "max": 1}], "confine_dopo": "parola_ascii_o_punto_cifra"}
      ],
      "revisioni": [{"id": "rev", "selettori": ["corpo"], "stato": "attiva", "sorgente": "inline", "separatori": ["/"],
                     "segmenti": [{"nome": "forte", "pattern": "[0-9]", "significato": "forte"},
                                  {"nome": "debole", "pattern": "[0-9]", "significato": "debole"}],
                     "token_sospesi": [{"id": "xx", "pattern": "xx", "riserva": "Q-ACME-1"}]}],
      "esempi": [
        {"id": "e1", "origine": "sintetico", "selettore": "corpo", "testo": "8.123.4/01",
         "atteso": {"letture": [{"forma": "completa", "base": "8.123.4", "revisione": "01"}]}},
        {"id": "e2", "origine": "sintetico", "selettore": "corpo", "testo": "8.123",
         "atteso": {"letture": [{"forma": "parziale", "mancanti": ["T"]}]}}
      ]
    },
    {
      "id": "acme-etichetta",
      "namespace": "acme",
      "ruoli": ["prodotto", "componente"],
      "base": {"segmenti": [{"nome": "numero", "pattern": "7[0-9]{6}", "identitario": true}],
               "maiuscole": "esatte", "normalizza": "nessuna", "confine_prima": "alnum_ascii", "confine_dopo": "alnum_ascii"},
      "etichette": [{"id": "pn", "letterali": ["PN"], "spazi_max": 1, "selettori": ["corpo", "oggetto"], "obbligatoria": true}],
      "forme": [{"id": "pn", "selettori": ["corpo", "oggetto"], "stato": "attiva", "completa": true,
                 "parti": [{"tipo": "etichetta", "rif": "pn", "min": 1, "max": 1}, {"tipo": "base", "min": 1, "max": 1}]}],
      "esempi": [{"id": "e1", "origine": "sintetico", "selettore": "corpo", "testo": "PN 7654321",
                  "atteso": {"letture": [{"forma": "pn", "base": "7654321"}]}}]
    },
    {
      "id": "acme-documento",
      "namespace": "acme",
      "ruoli": ["prodotto", "componente"],
      "base": {"segmenti": [{"nome": "numero", "pattern": "6[0-9]{6}", "identitario": true}],
               "maiuscole": "esatte", "normalizza": "nessuna"},
      "forme": [{"id": "documento", "selettori": ["nome_file"], "stato": "attiva", "completa": true,
                 "parti": [{"tipo": "base", "min": 1, "max": 1},
                           {"tipo": "decorazione", "rif": "suffisso", "min": 0, "max": 1},
                           {"tipo": "token", "letterali": ["00"], "rimando": "Q-ACME-1", "min": 0, "max": 1},
                           {"tipo": "decorazione", "rif": "stato", "min": 0, "max": 1}]}],
      "revisioni": [{"id": "rev-cartiglio", "selettori": ["cartiglio.revisione"], "stato": "attiva", "sorgente": "campo_separato",
                     "segmenti": [{"nome": "numero", "pattern": "[0-9]{2}", "significato": "nessuno"}]}],
      "decorazioni": [
        {"id": "suffisso", "tipo": "suffisso_documento", "selettori": ["nome_file"],
         "parti": [{"tipo": "separatore", "letterali": ["_"], "min": 1, "max": 1}, {"tipo": "ripetizione_base", "min": 1, "max": 1}]},
        {"id": "stato", "tipo": "stato_pdm", "letterali": ["IN_WORK"], "selettori": ["nome_file"]}
      ],
      "esempi": [
        {"id": "e1", "origine": "sintetico", "selettore": "nome_file", "testo": "6123456_6123456.pdf",
         "atteso": {"letture": [{"forma": "documento", "base": "6123456", "decorazioni": ["suffisso"]}]}},
        {"id": "e2", "origine": "sintetico", "selettore": "cartiglio.revisione", "testo": "03",
         "atteso": {"letture": [{"revisione": "03"}]}}
      ]
    }
  ]
}`

// limitiACME: limiti sintetici, con i valori iniziali del piano e una versione inventata. Nelle prove i
// limiti arrivano così, come li darebbe l'indice (R43 B).
func limitiACME() Limiti {
	return Limiti{
		Versione: "limiti-acme-1",
		Grammatica: LimitiGrammatica{
			MaxFamiglie: 20, MaxFormePerFamiglia: 16, MaxPartiPerForma: 12, MaxEsempi: 64,
			MaxLunghezzaPattern: 64, MaxRipetizione: 40, MaxLunghezzaLetterale: 40,
		},
		Riconoscimento: LimitiRiconoscimento{
			MaxByteUnita: 1048576, MaxUnitaDocumento: 10000, MaxLetturePerUnita: 1000, MaxLettureDocumento: 20000,
		},
		Anteprima: LimitiAnteprima{TempoMassimoMs: 20000},
	}
}

// variante: la grammatica ACME con delle sostituzioni, a coppie (vecchio, nuovo). Ogni vecchio deve
// comparire esattamente una volta: una prova non cambia per sbaglio un altro pezzo.
func variante(t *testing.T, sostituzioni ...string) []byte {
	t.Helper()
	if len(sostituzioni)%2 != 0 {
		t.Fatal("variante: le sostituzioni vanno a coppie")
	}
	s := grammaticaACME
	for i := 0; i < len(sostituzioni); i += 2 {
		if n := strings.Count(s, sostituzioni[i]); n != 1 {
			t.Fatalf("variante: %q compare %d volte, non una", sostituzioni[i], n)
		}
		s = strings.Replace(s, sostituzioni[i], sostituzioni[i+1], 1)
	}
	return []byte(s)
}

// diagnosiDi decodifica e valida: le diagnostiche della decodifica (errore) o della validazione.
func diagnosiDi(t *testing.T, raw []byte, lim Limiti) []evidenze.Diagnostica {
	t.Helper()
	g, err := Decodifica(raw)
	if err != nil {
		var ec *evidenze.ErroreContratto
		if !errors.As(err, &ec) {
			t.Fatalf("Decodifica: errore %T, atteso *evidenze.ErroreContratto: %v", err, err)
		}
		return ec.Diagnostiche
	}
	return g.Valida(lim)
}

func errori(ds []evidenze.Diagnostica) []evidenze.Diagnostica {
	var out []evidenze.Diagnostica
	for _, d := range ds {
		if d.Gravita == evidenze.GravitaErrore {
			out = append(out, d)
		}
	}
	return out
}

func elenca(ds []evidenze.Diagnostica) string {
	var b strings.Builder
	for _, d := range ds {
		fmt.Fprintf(&b, "\n  %s %s [%s] %s", d.Gravita, d.Codice, d.Percorso, d.Messaggio)
	}
	return b.String()
}

// richiedi controlla che fra le diagnostiche ce ne sia una con quel codice e quel percorso (vuoto: uno
// qualunque), e la restituisce.
func richiedi(t *testing.T, ds []evidenze.Diagnostica, codice, percorso string) evidenze.Diagnostica {
	t.Helper()
	for _, d := range ds {
		if d.Codice == codice && (percorso == "" || d.Percorso == percorso) {
			return d
		}
	}
	t.Fatalf("manca %s [%s]; diagnostiche:%s", codice, percorso, elenca(ds))
	return evidenze.Diagnostica{}
}

// mutata decodifica la grammatica ACME, la cambia in Go e la valida: per i casi che una sostituzione nel
// testo renderebbe fragili.
func mutata(t *testing.T, cambia func(*Grammatica)) []evidenze.Diagnostica {
	t.Helper()
	g, err := Decodifica([]byte(grammaticaACME))
	if err != nil {
		t.Fatal(err)
	}
	cambia(&g)
	return g.Valida(limitiACME())
}

// TestA1aGrammaticaACMEValida: la grammatica di partenza non ha errori; l'unica diagnostica è la nota della
// sua riserva. Se questa prova fallisce, le altre non dicono niente.
func TestA1aGrammaticaACMEValida(t *testing.T) {
	ds := diagnosiDi(t, []byte(grammaticaACME), limitiACME())
	if len(ds) != 1 {
		t.Fatalf("attesa solo la nota della riserva; diagnostiche:%s", elenca(ds))
	}
	d := ds[0]
	if d.Codice != CodiceRiserva || d.Gravita != evidenze.GravitaNota || d.Natura != evidenze.NaturaCapacita || d.Percorso != "profilo.riserve[Q-ACME-1]" {
		t.Fatalf("nota della riserva sbagliata:%s", elenca(ds))
	}
	if strings.Join(d.Rif, ",") != "Q-ACME-1,xx" {
		t.Fatalf("Rif della riserva = %v, attesi l'ID e le regole toccate", d.Rif)
	}
}

// casoErrore: una variante della grammatica ACME e l'errore che deve dare.
type casoErrore struct {
	nome         string
	sostituzioni []string
	codice       string
	percorso     string
}

// casiAC01: A-C01 (parte 1 §12): enum ignoti, coppie vietate, versioni dello schema, ruoli vuoti. Li usa
// anche la prova dello snapshot: con uno di questi errori lo snapshot non nasce.
var casiAC01 = []casoErrore{
	{"ruolo finito", []string{`"ruoli": ["componente"]`, `"ruoli": ["finito"]`}, CodiceEnumIgnoto, "famiglie_codice[acme-punti].ruoli[0]"},
	{"ruolo parte, il legacy", []string{`"ruoli": ["componente"]`, `"ruoli": ["parte"]`}, CodiceEnumIgnoto, "famiglie_codice[acme-punti].ruoli[0]"},
	{"categoria commerciale", []string{`"categorie": ["minuteria"]`, `"categorie": ["commerciale"]`}, CodiceEnumIgnoto, "famiglie_codice[acme-punti].categorie[0]"},
	{"tipo di parte inventato", []string{`"parti": [{"tipo": "base", "min": 1, "max": 1}], "confine_dopo"`, `"parti": [{"tipo": "base", "min": 1, "max": 1}, {"tipo": "coda", "min": 0, "max": 1}], "confine_dopo"`}, CodiceEnumIgnoto, "famiglie_codice[acme-punti].forme[parziale].parti[1].tipo"},
	{"tipo di decorazione inventato", []string{`"tipo": "stato_pdm"`, `"tipo": "colore"`}, CodiceEnumIgnoto, "famiglie_codice[acme-documento].decorazioni[stato].tipo"},
	{"posizione centro", []string{`"posizione": "prefisso"`, `"posizione": "centro"`}, CodiceEnumIgnoto, "famiglie_codice[acme-prefisso].affissi[fase].posizione"},
	{"classe di confine inventata", []string{`"confine_prima": "parola_ascii"`, `"confine_prima": "spazio_e_basta"`}, CodiceEnumIgnoto, "famiglie_codice[acme-punti].base.confine_prima"},
	{"coppia vietata: cartiglio con un campo STEP", []string{`"selettori": ["cartiglio.codice"]`, `"selettori": ["cartiglio.id"]`}, evidenze.CodiceSelettoreNonAmmesso, "famiglie_codice[acme-prefisso].forme[cartiglio].selettori[0]"},
	{"coppia vietata: nome_file con un campo del cartiglio", []string{`{"id": "nome", "selettori": ["nome_file"]`, `{"id": "nome", "selettori": ["nome_file.codice"]`}, evidenze.CodiceSelettoreNonAmmesso, "famiglie_codice[acme-prefisso].forme[nome].selettori[0]"},
	{"versione_schema 0", []string{`"versione_schema": 1`, `"versione_schema": 0`}, CodiceVersioneSchemaIgnota, "versione_schema"},
	{"versione_schema 2", []string{`"versione_schema": 1`, `"versione_schema": 2`}, CodiceVersioneSchemaIgnota, "versione_schema"},
	{"versione_schema assente", []string{`"versione_schema": 1,`, ``}, CodiceVersioneSchemaIgnota, "versione_schema"},
	{"ruoli vuoti", []string{`"ruoli": ["componente"]`, `"ruoli": []`}, CodiceRuoliVuoti, "famiglie_codice[acme-punti].ruoli"},
	{"ruoli assenti", []string{`"ruoli": ["componente"],`, ``}, CodiceRuoliVuoti, "famiglie_codice[acme-punti].ruoli"},
}

// TestAC01ErroriEspliciti: ogni caso dà il suo errore, con codice e percorso, gravità errore; nessun
// ripiego (un ruolo legacy non diventa un ruolo del motore A).
func TestAC01ErroriEspliciti(t *testing.T) {
	for _, c := range casiAC01 {
		t.Run(c.nome, func(t *testing.T) {
			ds := diagnosiDi(t, variante(t, c.sostituzioni...), limitiACME())
			d := richiedi(t, ds, c.codice, c.percorso)
			if d.Gravita != evidenze.GravitaErrore || d.Natura != evidenze.NaturaContratto {
				t.Fatalf("%s: gravità %s e natura %s, attesi errore di contratto", c.codice, d.Gravita, d.Natura)
			}
			if d.Messaggio == "" {
				t.Fatal("errore senza messaggio")
			}
		})
	}
}

// TestAC01SchemaIgnotoNonGuardaAltro: con una versione ignota la decodifica dà solo quella diagnosi, anche
// se il file ha chiavi che la versione 1 non conosce: potrebbero essere giuste per un'altra versione.
func TestAC01SchemaIgnotoNonGuardaAltro(t *testing.T) {
	ds := diagnosiDi(t, variante(t, `"versione_schema": 1,`, `"versione_schema": 2, "novita": true,`), limitiACME())
	if len(ds) != 1 || ds[0].Codice != CodiceVersioneSchemaIgnota {
		t.Fatalf("attesa solo %s; diagnostiche:%s", CodiceVersioneSchemaIgnota, elenca(ds))
	}
}

// TestA1aValidaRegoleDelContratto: le regole 1-15 della validazione, un caso per regola (o per ramo), con
// il codice e il percorso attesi.
func TestA1aValidaRegoleDelContratto(t *testing.T) {
	casi := []casoErrore{
		// 1. ID
		{"id vuoto", []string{`{"id": "pn", "selettori"`, `{"id": "", "selettori"`}, CodiceIDVuoto, "famiglie_codice[acme-etichetta].forme[0]"},
		{"id ripetuto fra le forme", []string{`{"id": "cartiglio", "selettori"`, `{"id": "mail", "selettori"`}, CodiceIDRipetuto, "famiglie_codice[acme-prefisso].forme[mail]"},
		{"famiglia ripetuta", []string{`"id": "acme-etichetta"`, `"id": "acme-punti"`}, CodiceIDRipetuto, "famiglie_codice[acme-punti]"},
		{"segmento della base ripetuto", []string{`{"nome": "b", "pattern"`, `{"nome": "a", "pattern"`}, CodiceIDRipetuto, "famiglie_codice[acme-punti].base.segmenti[a]"},
		// 2. rif
		{"rif pendente", []string{`{"tipo": "affisso", "rif": "fase", "min": 0, "max": 1}, {"tipo": "base"`, `{"tipo": "affisso", "rif": "fasi", "min": 0, "max": 1}, {"tipo": "base"`}, CodiceRiferimentoPendente, "famiglie_codice[acme-prefisso].forme[mail].parti[0].rif"},
		{"rif del tipo sbagliato", []string{`{"tipo": "revisione", "rif": "rev"`, `{"tipo": "decorazione", "rif": "rev"`}, CodiceRiferimentoPendente, "famiglie_codice[acme-punti].forme[completa].parti[1].rif"},
		{"revisione in campo separato dentro una forma", []string{`{"tipo": "token", "letterali": ["00"]`, `{"tipo": "revisione", "rif": "rev-cartiglio", "min": 0, "max": 1}, {"tipo": "token", "letterali": ["00"]`}, CodiceRiferimentoPendente, "famiglie_codice[acme-documento].forme[documento].parti[2].rif"},
		{"lettura attesa con una forma che non c'è", []string{`"atteso": {"letture": [{"forma": "cartiglio"}]}`, `"atteso": {"letture": [{"forma": "disegno"}]}`}, CodiceRiferimentoPendente, "famiglie_codice[acme-prefisso].esempi[e3].atteso.letture[0].forma"},
		{"lettura attesa con una famiglia che non c'è", []string{`"atteso": {"letture": [{"forma": "cartiglio"}]}`, `"atteso": {"letture": [{"famiglia": "acme-altro", "forma": "cartiglio"}]}`}, CodiceRiferimentoPendente, "famiglie_codice[acme-prefisso].esempi[e3].atteso.letture[0].famiglia"},
		// 3. enum
		{"stato della forma inventato", []string{`{"id": "pn", "selettori": ["corpo", "oggetto"], "stato": "attiva"`, `{"id": "pn", "selettori": ["corpo", "oggetto"], "stato": "accesa"`}, CodiceEnumIgnoto, "famiglie_codice[acme-etichetta].forme[pn].stato"},
		{"sorgente inventata", []string{`"sorgente": "inline"`, `"sorgente": "allegato"`}, CodiceEnumIgnoto, "famiglie_codice[acme-punti].revisioni[rev].sorgente"},
		{"significato inventato", []string{`"significato": "debole"`, `"significato": "medio"`}, CodiceEnumIgnoto, "famiglie_codice[acme-punti].revisioni[rev].segmenti[debole].significato"},
		{"fase inventata", []string{`"valore": {"fase": "prototipo"}`, `"valore": {"fase": "beta"}`}, CodiceEnumIgnoto, "famiglie_codice[acme-prefisso].affissi[fase].valore.fase"},
		{"destinazione inventata", []string{`"valore": {"fase": "prototipo"}`, `"valore": {"destinazione": "magazzino"}`}, CodiceEnumIgnoto, "famiglie_codice[acme-prefisso].affissi[fase].valore.destinazione"},
		{"maiuscole inventate", []string{`"maiuscole": "esatte", "normalizza": "nessuna"},`, `"maiuscole": "grandi", "normalizza": "nessuna"},`}, CodiceEnumIgnoto, "famiglie_codice[acme-documento].base.maiuscole"},
		{"normalizzazione inventata", []string{`"maiuscole": "esatte", "normalizza": "nessuna"},`, `"maiuscole": "esatte", "normalizza": "minuscolo"},`}, CodiceEnumIgnoto, "famiglie_codice[acme-documento].base.normalizza"},
		{"origine dell'esempio inventata", []string{`{"id": "e4", "origine": "sintetico"`, `{"id": "e4", "origine": "inventata"`}, CodiceEnumIgnoto, "famiglie_codice[acme-prefisso].esempi[e4].origine"},
		{"stato del profilo inventato", []string{`"stato": "parziale"`, `"stato": "quasi"`}, CodiceEnumIgnoto, "profilo.stato"},
		{"sottotipo dell'involucro assente", []string{`"sottotipo": "riferimento_pacchetto",`, ``}, CodiceEnumIgnoto, "famiglie_codice[acme-prefisso].decorazioni[pacchetto].sottotipo"},
		{"sottotipo fuori da un involucro", []string{`{"id": "stato", "tipo": "stato_pdm",`, `{"id": "stato", "tipo": "stato_pdm", "sottotipo": "tecnico",`}, CodiceEnumIgnoto, "famiglie_codice[acme-documento].decorazioni[stato].sottotipo"},
		{"parte interna non ammessa nel suffisso", []string{`{"tipo": "ripetizione_base", "min": 1, "max": 1}]},`, `{"tipo": "base", "min": 1, "max": 1}]},`}, CodiceEnumIgnoto, "famiglie_codice[acme-documento].decorazioni[suffisso].parti[1].tipo"},
		{"letterale e pattern insieme", []string{`{"nome": "a", "letterale": "8",`, `{"nome": "a", "letterale": "8", "pattern": "8",`}, CodiceEnumIgnoto, "famiglie_codice[acme-punti].base.segmenti[a]"},
		{"rimando fuori da un token", []string{`{"tipo": "etichetta", "rif": "pn", "min": 1, "max": 1}`, `{"tipo": "etichetta", "rif": "pn", "rimando": "Q1", "min": 1, "max": 1}`}, CodiceEnumIgnoto, "famiglie_codice[acme-etichetta].forme[pn].parti[0].rimando"},
		// 4. ruoli e insiemi
		{"selettore ripetuto", []string{`"selettori": ["corpo", "storia"]`, `"selettori": ["corpo", "corpo"]`}, CodiceInsiemeConDuplicati, "famiglie_codice[acme-prefisso].forme[mail].selettori[1]"},
		{"letterale ripetuto", []string{`"letterali": ["PN"]`, `"letterali": ["PN", "PN"]`}, CodiceInsiemeConDuplicati, "famiglie_codice[acme-etichetta].etichette[pn].letterali[1]"},
		{"ruolo ripetuto", []string{`"ruoli": ["componente"]`, `"ruoli": ["componente", "componente"]`}, CodiceInsiemeConDuplicati, "famiglie_codice[acme-punti].ruoli[1]"},
		// 6. attribuzione
		{"attribuzione fuori dal riconoscimento", []string{`"attribuzione": ["corpo", "storia", "nome_file"]`, `"attribuzione": ["corpo", "oggetto"]`}, CodiceAttribuzioneFuoriRiconoscimento, "famiglie_codice[acme-prefisso].affissi[fase].attribuzione[1]"},
		// 7. parti sui selettori della forma
		{"parte fuori selettore", []string{`"riconoscimento": ["corpo", "storia", "nome_file"]`, `"riconoscimento": ["corpo", "nome_file"]`}, CodiceParteFuoriSelettore, "famiglie_codice[acme-prefisso].forme[mail].parti[0]"},
		// 8. forme parziali
		{"parziale senza mancanti", []string{`"completa": false, "segmenti_mancanti": ["T"],`, `"completa": false,`}, CodiceFormaParzialeNonProiezione, "famiglie_codice[acme-punti].forme[parziale].segmenti_mancanti"},
		{"completa con mancanti", []string{`"completa": false, "segmenti_mancanti": ["T"],`, `"completa": true, "segmenti_mancanti": ["T"],`}, CodiceFormaParzialeNonProiezione, "famiglie_codice[acme-punti].forme[parziale].segmenti_mancanti"},
		{"mancante che non è della base", []string{`"segmenti_mancanti": ["T"]`, `"segmenti_mancanti": ["Z"]`}, CodiceFormaParzialeNonProiezione, "famiglie_codice[acme-punti].forme[parziale].segmenti_mancanti[0]"},
		{"mancante non identitario", []string{`{"nome": "T", "pattern": "[0-9]", "separatore": ".", "identitario": true}`, `{"nome": "T", "pattern": "[0-9]", "separatore": ".", "identitario": false}`}, CodiceFormaParzialeNonProiezione, "famiglie_codice[acme-punti].forme[parziale].segmenti_mancanti[0]"},
		// 9. cardinalità
		{"max 2", []string{`{"tipo": "revisione", "rif": "rev", "min": 0, "max": 1}`, `{"tipo": "revisione", "rif": "rev", "min": 0, "max": 2}`}, CodiceCardinalitaNonValida, "famiglie_codice[acme-punti].forme[completa].parti[1]"},
		{"base facoltativa", []string{`"parti": [{"tipo": "base", "min": 1, "max": 1}], "confine_dopo"`, `"parti": [{"tipo": "base", "min": 0, "max": 1}], "confine_dopo"`}, CodiceCardinalitaNonValida, "famiglie_codice[acme-punti].forme[parziale].parti[0]"},
		{"separatore facoltativo", []string{`{"tipo": "separatore", "letterali": ["_"], "min": 1,`, `{"tipo": "separatore", "letterali": ["_"], "min": 0,`}, CodiceCardinalitaNonValida, "famiglie_codice[acme-documento].decorazioni[suffisso].parti[0]"},
		{"due basi", []string{`"parti": [{"tipo": "base", "min": 1, "max": 1}], "confine_dopo"`, `"parti": [{"tipo": "base", "min": 1, "max": 1}, {"tipo": "base", "min": 1, "max": 1}], "confine_dopo"`}, CodiceCardinalitaNonValida, "famiglie_codice[acme-punti].forme[parziale].parti"},
		{"spazi_max negativo", []string{`"spazi_max": 1`, `"spazi_max": -1`}, CodiceCardinalitaNonValida, "famiglie_codice[acme-etichetta].etichette[pn].spazi_max"},
		// 10. etichetta obbligatoria (A-C08)
		{"etichetta obbligatoria facoltativa", []string{`{"tipo": "etichetta", "rif": "pn", "min": 1, "max": 1}`, `{"tipo": "etichetta", "rif": "pn", "min": 0, "max": 1}`}, CodiceEtichettaObbligatoriaAssente, "famiglie_codice[acme-etichetta].forme[pn]"},
		{"etichetta obbligatoria assente", []string{`"parti": [{"tipo": "etichetta", "rif": "pn", "min": 1, "max": 1}, {"tipo": "base", "min": 1, "max": 1}]`, `"parti": [{"tipo": "base", "min": 1, "max": 1}]`}, CodiceEtichettaObbligatoriaAssente, "famiglie_codice[acme-etichetta].forme[pn]"},
		// 11. selettore conteso
		{"forme e campo separato sullo stesso selettore", []string{`"selettori": ["cartiglio.revisione"]`, `"selettori": ["nome_file"]`}, CodiceSelettoreConteso, "famiglie_codice[acme-documento].revisioni[rev-cartiglio].selettori[0]"},
		// 12. confini
		{"punto_cifra a sinistra nella base", []string{`"confine_prima": "parola_ascii"`, `"confine_prima": "parola_ascii_o_punto_cifra"`}, CodiceConfineNonAmmesso, "famiglie_codice[acme-punti].base.confine_prima"},
		{"punto_cifra a sinistra nella forma", []string{`"confine_dopo": "parola_ascii_o_punto_cifra"`, `"confine_prima": "parola_ascii_o_punto_cifra"`}, CodiceConfineNonAmmesso, "famiglie_codice[acme-punti].forme[parziale].confine_prima"},
		// 13. letterali (i pattern hanno la loro prova)
		{"letterale vuoto", []string{`"letterali": ["PN"]`, `"letterali": [""]`}, CodiceLetteraleVuoto, "famiglie_codice[acme-etichetta].etichette[pn].letterali[0]"},
		{"spazio unificatore in un letterale", []string{`"letterali": ["PN"]`, `"letterali": ["P N"]`}, CodiceCarattereDiControllo, "famiglie_codice[acme-etichetta].etichette[pn].letterali[0]"},
		{"carattere di formato in un letterale", []string{`"letterali": ["_"]`, `"letterali": ["_​"]`}, CodiceCarattereDiControllo, "famiglie_codice[acme-documento].decorazioni[suffisso].parti[0].letterali[0]"},
		// 15. esempi
		{"forma attiva senza esempio positivo", []string{`"atteso": {"letture": [{"forma": "cartiglio"}]}`, `"atteso": {"nessuna": true}`}, CodiceCampoObbligatorio, "famiglie_codice[acme-prefisso].forme[cartiglio]"},
		{"esempio senza atteso", []string{`"atteso": {"letture": [{"forma": "cartiglio"}]}`, `"atteso": {}`}, CodiceCampoObbligatorio, "famiglie_codice[acme-prefisso].esempi[e3].atteso"},
		{"nessuna e letture insieme", []string{`"atteso": {"letture": [{"forma": "cartiglio"}]}`, `"atteso": {"nessuna": true, "letture": [{"forma": "cartiglio"}]}`}, CodiceEnumIgnoto, "famiglie_codice[acme-prefisso].esempi[e3].atteso"},
		// campi obbligatori
		{"token senza rimando", []string{`"rimando": "Q-ACME-1",`, ``}, CodiceCampoObbligatorio, "famiglie_codice[acme-documento].forme[documento].parti[2].rimando"},
		{"separatore senza letterali", []string{`{"tipo": "separatore", "letterali": ["_"], "min": 1, "max": 1}`, `{"tipo": "separatore", "min": 1, "max": 1}`}, CodiceCampoObbligatorio, "famiglie_codice[acme-documento].decorazioni[suffisso].parti[0].letterali"},
		{"revisione senza segmenti né token", []string{`"segmenti": [{"nome": "numero", "pattern": "[0-9]{2}", "significato": "nessuno"}]`, `"segmenti": []`}, CodiceCampoObbligatorio, "famiglie_codice[acme-documento].revisioni[rev-cartiglio].segmenti"},
		{"cliente senza UUID", []string{`"id": "00000000-0000-4000-8000-00000000ac01"`, `"id": "00000000-0000-0000-0000-000000000000"`}, CodiceCampoObbligatorio, "cliente.id"},
		{"suffisso con letterali al posto delle parti", []string{`"parti": [{"tipo": "separatore", "letterali": ["_"], "min": 1, "max": 1}, {"tipo": "ripetizione_base", "min": 1, "max": 1}]`, `"letterali": ["_"]`}, CodiceEnumIgnoto, "famiglie_codice[acme-documento].decorazioni[suffisso]"},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			ds := diagnosiDi(t, variante(t, c.sostituzioni...), limitiACME())
			d := richiedi(t, ds, c.codice, c.percorso)
			if d.Gravita != evidenze.GravitaErrore {
				t.Fatalf("%s con gravità %s, atteso errore", c.codice, d.Gravita)
			}
		})
	}
}

// TestA1aValidaSenzaRipieghiNeDoppiSensi: una revisione in campo separato senza forte e debole (D-06 per il
// token, D-07 per l'esempio) è valida; il selettore «storia» è attivo come gli altri (R48 A); una famiglia
// con sole forme riservate è ammessa, senza esempi positivi obbligatori.
func TestA1aValidaSenzaRipieghiNeDoppiSensi(t *testing.T) {
	t.Run("revisione con il solo token sospeso", func(t *testing.T) {
		ds := mutata(t, func(g *Grammatica) { g.Famiglie[1].Revisioni[0].Segmenti = nil })
		if e := errori(ds); len(e) > 0 {
			t.Fatalf("errori inattesi:%s", elenca(e))
		}
	})
	t.Run("famiglia senza esempi", func(t *testing.T) {
		ds := mutata(t, func(g *Grammatica) { g.Famiglie[2].Esempi = nil })
		richiedi(t, ds, CodiceCampoObbligatorio, "famiglie_codice[acme-etichetta].esempi")
	})
	t.Run("famiglia con sole forme riservate", func(t *testing.T) {
		raw := variante(t, `{"id": "pn", "selettori": ["corpo", "oggetto"], "stato": "attiva"`, `{"id": "pn", "selettori": ["corpo", "oggetto"], "stato": "riservata"`,
			`"atteso": {"letture": [{"forma": "pn", "base": "7654321"}]}`, `"atteso": {"nessuna": true}`)
		ds := diagnosiDi(t, raw, limitiACME())
		if e := errori(ds); len(e) > 0 {
			t.Fatalf("errori inattesi:%s", elenca(e))
		}
		d := richiedi(t, ds, CodiceFormaRiservata, "famiglie_codice[acme-etichetta].forme[pn]")
		if d.Gravita != evidenze.GravitaNota || d.Natura != evidenze.NaturaCapacita {
			t.Fatalf("forma riservata: %s %s, attesa una nota di capacità", d.Gravita, d.Natura)
		}
	})
	t.Run("validazione deterministica", func(t *testing.T) {
		raw := variante(t, `"ruoli": ["componente"]`, `"ruoli": ["finito", "parte"]`)
		a, b := diagnosiDi(t, raw, limitiACME()), diagnosiDi(t, raw, limitiACME())
		if elenca(a) != elenca(b) {
			t.Fatalf("due validazioni diverse:%s\n---%s", elenca(a), elenca(b))
		}
	})
}
