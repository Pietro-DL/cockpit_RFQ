// L1 — la colonna della quantità nel file regole (A1b-20; piano A, 5.4.7; R28 a): quantita_tabellare si legge
// con la porta stretta, si valida con un codice e un percorso per ogni violazione, ha una forma canonica che non
// dipende dall'ordine, e non cambia il canonico né l'hash dei file che non la usano.
package grammatica

import (
	"bytes"
	"strings"
	"testing"

	"promatec/cockpit/internal/core/estrazione/evidenze"
)

// I clienti di questi test sono inventati. Non è pigrizia: le famiglie di codice dei clienti veri sono un dato
// dell'azienda e questo repository è pubblico, e una prova che dipendesse da esse diventerebbe rossa il giorno in
// cui un cliente cambia convenzione. La grammatica è quella ACME di valida_test.go, con una regola della quantità
// in più; il letterale dell'intestazione è inventato («Q.TA»), mai quello di un cliente. Le prove citano i
// requisiti (A1b-20, R28 a), mai i casi degli attesi.

// hashACMEDiA1a: l'hash dello snapshot della grammatica ACME di valida_test.go calcolato con il codice di A1a,
// prima che quantita_tabellare esistesse (sorgenti di grammatica, evidenze e jsoncanonico del commit di A1b.7,
// con gli stessi limiti). Un file che non usa il campo deve averlo identico (5.4.7: «con omitempty, il canonico
// e l'hash dei file che non usano il campo non cambiano»).
const hashACMEDiA1a = "929397515c5af8feb2b825d0a115ad32940b9819dec13e15455c76c96c0b53c7"

// regolaQuantitaACME: una regola valida, attiva su corpo e storia (R48 A: la tabella di una mail inoltrata senza
// confine sta nella storia).
const regolaQuantitaACME = `{"id": "q-acme", "intestazioni": ["Q.TA"], "regola": "prima_riga_non_vuota_sopra_le_righe_con_codice", "selettori": ["corpo", "storia"], "stato": "attiva"}`

// conQuantita: la grammatica ACME con l'elenco quantita_tabellare dato (il testo JSON dell'array).
func conQuantita(t *testing.T, elenco string) []byte {
	t.Helper()
	return variante(t, `"famiglie_codice": [`, `"quantita_tabellare": `+elenco+`,
  "famiglie_codice": [`)
}

func TestLaColonnaQuantitaSiDichiaraNelFileRegole(t *testing.T) {
	t.Run("una regola valida si legge e non porta diagnostiche nuove", func(t *testing.T) {
		raw := conQuantita(t, `[`+regolaQuantitaACME+`]`)
		g, err := Decodifica(raw)
		if err != nil {
			t.Fatalf("Decodifica: %v", err)
		}
		if len(g.Quantita) != 1 {
			t.Fatalf("Quantita = %+v, attesa una regola", g.Quantita)
		}
		q := g.Quantita[0]
		if q.ID != "q-acme" || strings.Join(q.Intestazioni, "|") != "Q.TA" || q.Regola != RegolaQuantitaPrimaRigaSopra ||
			strings.Join(q.Selettori, "|") != "corpo|storia" || q.Stato != StatoAttiva {
			t.Fatalf("regola letta male: %+v", q)
		}
		if RegolaQuantitaPrimaRigaSopra != "prima_riga_non_vuota_sopra_le_righe_con_codice" {
			t.Fatalf("il nome della regola è un dato del contratto: %q", RegolaQuantitaPrimaRigaSopra)
		}
		// La sola diagnostica resta la nota della riserva della grammatica ACME.
		ds := g.Valida(limitiACME())
		if len(ds) != 1 || ds[0].Codice != CodiceRiserva {
			t.Fatalf("diagnostiche:%s", elenca(ds))
		}
		// Una regola riservata è valida: resta nel file e nello snapshot, e non si attiva (v3 §2).
		riservata := strings.Replace(regolaQuantitaACME, `"stato": "attiva"`, `"stato": "riservata"`, 1)
		if ds := errori(diagnosiDi(t, conQuantita(t, `[`+riservata+`]`), limitiACME())); len(ds) != 0 {
			t.Fatalf("una regola riservata non è un errore:%s", elenca(ds))
		}
	})

	t.Run("la decodifica è stretta anche dentro quantita_tabellare", func(t *testing.T) {
		casi := []struct {
			nome, elenco, codice, percorso string
		}{
			{"chiave sconosciuta", `[{"id": "q-acme", "colonna": 3, "intestazioni": ["Q.TA"], "regola": "prima_riga_non_vuota_sopra_le_righe_con_codice", "selettori": ["corpo"], "stato": "attiva"}]`,
				CodiceChiaveSconosciuta, "quantita_tabellare[0].colonna"},
			{"chiave con le maiuscole diverse", `[{"ID": "q-acme", "intestazioni": ["Q.TA"], "regola": "prima_riga_non_vuota_sopra_le_righe_con_codice", "selettori": ["corpo"], "stato": "attiva"}]`,
				CodiceChiaveMaiuscole, "quantita_tabellare[0].ID"},
			{"chiave ripetuta", `[{"id": "q-acme", "id": "q-acme", "intestazioni": ["Q.TA"], "regola": "prima_riga_non_vuota_sopra_le_righe_con_codice", "selettori": ["corpo"], "stato": "attiva"}]`,
				CodiceChiaveRipetuta, "quantita_tabellare[0].id"},
			{"null al posto delle intestazioni", `[{"id": "q-acme", "intestazioni": null, "regola": "prima_riga_non_vuota_sopra_le_righe_con_codice", "selettori": ["corpo"], "stato": "attiva"}]`,
				CodiceNull, "quantita_tabellare[0].intestazioni"},
			{"null in un elenco", `[{"id": "q-acme", "intestazioni": ["Q.TA"], "regola": "prima_riga_non_vuota_sopra_le_righe_con_codice", "selettori": [null], "stato": "attiva"}]`,
				CodiceNull, "quantita_tabellare[0].selettori[0]"},
			{"null al posto dell'elenco", `null`, CodiceNull, "quantita_tabellare"},
		}
		for _, c := range casi {
			t.Run(c.nome, func(t *testing.T) {
				ds := erroreDecodifica(t, conQuantita(t, c.elenco))
				richiedi(t, ds, c.codice, c.percorso)
			})
		}
		// Un'intestazione scritta come stringa invece che come elenco non si legge: nessun ripiego verso
		// «un elenco di un elemento».
		ds := erroreDecodifica(t, conQuantita(t, `[{"id": "q-acme", "intestazioni": "Q.TA", "regola": "prima_riga_non_vuota_sopra_le_righe_con_codice", "selettori": ["corpo"], "stato": "attiva"}]`))
		if len(ds) == 0 {
			t.Fatal("un'intestazione stringa deve essere un errore di decodifica")
		}
	})

	t.Run("la validazione dà un codice e un percorso per ogni violazione", func(t *testing.T) {
		regola := func(id, intestazioni, regola, selettori, stato string) string {
			return `{"id": "` + id + `", "intestazioni": ` + intestazioni + `, "regola": "` + regola + `", "selettori": ` + selettori + `, "stato": "` + stato + `"}`
		}
		const r = "prima_riga_non_vuota_sopra_le_righe_con_codice"
		// Il carattere di controllo si scrive con l'escape JSON, composto qui: il testo del file non lo contiene.
		bell := `"Q.` + `\` + `u0007TA"`
		lungo := `"` + strings.Repeat("Q", 41) + `"`
		casi := []struct {
			nome, elenco, codice, percorso string
		}{
			{"ID vuoto", `[` + regola("", `["Q.TA"]`, r, `["corpo"]`, "attiva") + `]`, CodiceIDVuoto, "quantita_tabellare[0]"},
			{"ID ripetuto", `[` + regola("q-acme", `["Q.TA"]`, r, `["corpo"]`, "attiva") + `, ` + regola("q-acme", `["PEZZI"]`, r, `["storia"]`, "attiva") + `]`,
				CodiceIDRipetuto, "quantita_tabellare[q-acme]"},
			{"nessuna intestazione", `[` + regola("q-acme", `[]`, r, `["corpo"]`, "attiva") + `]`, CodiceCampoObbligatorio, "quantita_tabellare[q-acme].intestazioni"},
			{"intestazione vuota", `[` + regola("q-acme", `[""]`, r, `["corpo"]`, "attiva") + `]`, CodiceLetteraleVuoto, "quantita_tabellare[q-acme].intestazioni[0]"},
			{"intestazione con un carattere di controllo", `[` + regola("q-acme", `[`+bell+`]`, r, `["corpo"]`, "attiva") + `]`,
				CodiceCarattereDiControllo, "quantita_tabellare[q-acme].intestazioni[0]"},
			{"intestazione oltre la lunghezza dei letterali", `[` + regola("q-acme", `[`+lungo+`]`, r, `["corpo"]`, "attiva") + `]`,
				CodiceLimiteSuperato, "quantita_tabellare[q-acme].intestazioni[0]"},
			{"intestazioni ripetute", `[` + regola("q-acme", `["Q.TA", "Q.TA"]`, r, `["corpo"]`, "attiva") + `]`,
				CodiceInsiemeConDuplicati, "quantita_tabellare[q-acme].intestazioni[1]"},
			{"regola ignota", `[` + regola("q-acme", `["Q.TA"]`, "colonna_piu_vicina", `["corpo"]`, "attiva") + `]`, CodiceEnumIgnoto, "quantita_tabellare[q-acme].regola"},
			{"regola vuota", `[` + regola("q-acme", `["Q.TA"]`, "", `["corpo"]`, "attiva") + `]`, CodiceEnumIgnoto, "quantita_tabellare[q-acme].regola"},
			{"nessun selettore", `[` + regola("q-acme", `["Q.TA"]`, r, `[]`, "attiva") + `]`, CodiceCampoObbligatorio, "quantita_tabellare[q-acme].selettori"},
			{"selettore oggetto", `[` + regola("q-acme", `["Q.TA"]`, r, `["oggetto"]`, "attiva") + `]`, CodiceEnumIgnoto, "quantita_tabellare[q-acme].selettori[0]"},
			{"selettore di un campo del cartiglio", `[` + regola("q-acme", `["Q.TA"]`, r, `["cartiglio.codice"]`, "attiva") + `]`,
				CodiceEnumIgnoto, "quantita_tabellare[q-acme].selettori[0]"},
			{"selettore dell'elenco del PDF", `[` + regola("q-acme", `["Q.TA"]`, r, `["elenco_pdf.quantita"]`, "attiva") + `]`,
				CodiceEnumIgnoto, "quantita_tabellare[q-acme].selettori[0]"},
			{"selettore fuori vocabolario", `[` + regola("q-acme", `["Q.TA"]`, r, `["tabella"]`, "attiva") + `]`,
				evidenze.CodiceSelettoreNonAmmesso, "quantita_tabellare[q-acme].selettori[0]"},
			{"selettore generico", `[` + regola("q-acme", `["Q.TA"]`, r, `["cartiglio.*"]`, "attiva") + `]`,
				evidenze.CodiceSelettoreGenerico, "quantita_tabellare[q-acme].selettori[0]"},
			{"selettori ripetuti", `[` + regola("q-acme", `["Q.TA"]`, r, `["corpo", "corpo"]`, "attiva") + `]`,
				CodiceInsiemeConDuplicati, "quantita_tabellare[q-acme].selettori[1]"},
			{"stato ignoto", `[` + regola("q-acme", `["Q.TA"]`, r, `["corpo"]`, "bozza") + `]`, CodiceEnumIgnoto, "quantita_tabellare[q-acme].stato"},
		}
		for _, c := range casi {
			t.Run(c.nome, func(t *testing.T) {
				ds := diagnosiDi(t, conQuantita(t, c.elenco), limitiACME())
				d := richiedi(t, ds, c.codice, c.percorso)
				if d.Gravita != evidenze.GravitaErrore {
					t.Fatalf("%s con gravità %s: una regola della quantità sbagliata è un errore, mai un ripiego:%s", c.codice, d.Gravita, elenca(ds))
				}
				// Con un errore lo snapshot non nasce (A-C01): nessuna regola sbagliata arriva al motore.
				if _, err := NuovoSnapshot(conQuantita(t, c.elenco), limitiACME()); err == nil {
					t.Fatal("snapshot nato con una regola della quantità non valida")
				}
			})
		}
	})

	t.Run("l'hash dei file che non usano il campo non cambia", func(t *testing.T) {
		s := snapshotDi(t, []byte(grammaticaACME), limitiACME())
		if s.Hash != hashACMEDiA1a {
			t.Fatalf("la grammatica ACME senza quantita_tabellare ha l'hash %s, con il codice di A1a era %s", s.Hash, hashACMEDiA1a)
		}
		if bytes.Contains(s.Canonico, []byte("quantita_tabellare")) {
			t.Fatal("il canonico di un file che non usa il campo nomina quantita_tabellare")
		}
		// Un elenco vuoto è un campo non usato: omitempty lo toglie dal canonico.
		if h := snapshotDi(t, conQuantita(t, `[]`), limitiACME()).Hash; h != hashACMEDiA1a {
			t.Fatalf("con quantita_tabellare vuoto l'hash cambia: %s", h)
		}
		// Un file che il campo lo usa ha un hash suo, e il canonico porta la regola.
		con := snapshotDi(t, conQuantita(t, `[`+regolaQuantitaACME+`]`), limitiACME())
		if con.Hash == hashACMEDiA1a || !bytes.Contains(con.Canonico, []byte(`"quantita_tabellare"`)) {
			t.Fatalf("una regola della quantità non entra nel canonico: %s", con.Hash)
		}
		// Il canonico si rilegge con la porta stretta e dà lo stesso hash.
		if h := snapshotDi(t, con.Canonico, limitiACME()).Hash; h != con.Hash {
			t.Fatalf("il canonico riletto cambia hash: %s, %s", h, con.Hash)
		}
	})

	t.Run("l'ordine di regole, intestazioni e selettori non conta, il contenuto sì", func(t *testing.T) {
		const r = "prima_riga_non_vuota_sopra_le_righe_con_codice"
		a := `[{"id": "q-acme", "intestazioni": ["Q.TA", "PEZZI"], "regola": "` + r + `", "selettori": ["corpo", "storia"], "stato": "attiva"},
		       {"id": "q-acme-2", "intestazioni": ["N."], "regola": "` + r + `", "selettori": ["storia"], "stato": "riservata"}]`
		b := `[{"id": "q-acme-2", "intestazioni": ["N."], "regola": "` + r + `", "selettori": ["storia"], "stato": "riservata"},
		       {"id": "q-acme", "intestazioni": ["PEZZI", "Q.TA"], "regola": "` + r + `", "selettori": ["storia", "corpo"], "stato": "attiva"}]`
		ha := snapshotDi(t, conQuantita(t, a), limitiACME()).Hash
		if hb := snapshotDi(t, conQuantita(t, b), limitiACME()).Hash; ha != hb {
			t.Fatalf("regole, intestazioni o selettori permutati cambiano l'hash: %s, %s", ha, hb)
		}
		diversi := map[string]string{
			"un'intestazione in più":                  strings.Replace(a, `["Q.TA", "PEZZI"]`, `["Q.TA", "PEZZI", "QTA"]`, 1),
			"un selettore in meno":                    strings.Replace(a, `"selettori": ["corpo", "storia"]`, `"selettori": ["storia"]`, 1),
			"lo stato":                                strings.Replace(a, `"stato": "attiva"`, `"stato": "riservata"`, 1),
			"l'intestazione con le maiuscole diverse": strings.Replace(a, `"Q.TA"`, `"Q.Ta"`, 1),
		}
		for _, nome := range []string{"un'intestazione in più", "un selettore in meno", "lo stato", "l'intestazione con le maiuscole diverse"} {
			if h := snapshotDi(t, conQuantita(t, diversi[nome]), limitiACME()).Hash; h == ha {
				t.Errorf("%s: stesso hash", nome)
			}
		}
		// Normalizza non cambia la grammatica ricevuta.
		g, err := Decodifica(conQuantita(t, b))
		if err != nil {
			t.Fatal(err)
		}
		_ = Normalizza(g)
		if g.Quantita[0].ID != "q-acme-2" || g.Quantita[1].Intestazioni[0] != "PEZZI" || g.Quantita[1].Selettori[0] != "storia" {
			t.Fatalf("Normalizza ha cambiato la grammatica ricevuta: %+v", g.Quantita)
		}
	})
}
