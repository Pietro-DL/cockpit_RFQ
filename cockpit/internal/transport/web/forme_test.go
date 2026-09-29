package web

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/registro/censimento"
	"promatec/cockpit/internal/platform/db"
)

// L1 — Smistamento, giro 4, fase 4.17a: la pagina «Forme viste» si esegue con un censimento ACME inventato (gli
// errori di campo dei template escono solo all'esecuzione) e mostra le sue sezioni: il riepilogo, le forme per
// fonte con le colonne delle fonti, l'alias candidato, le letture del nome diverse dalla decisione, le firme,
// la minuteria (con le proposte e senza), il link all'export. Nessun form, nessun bottone che manda
// qualcosa: la pagina legge.
func TestLaPaginaDelleFormeViste(t *testing.T) {
	s := serverTest(t)
	acme, t1, t2 := uuid.New(), uuid.New(), uuid.New()
	regoleACME := json.RawMessage(`{"famiglie_codice": [{"regex": "(?P<codice>712\\d{4})", "descrizione": "ACME 712", "esempio": "7120001"}]}`)
	in := censimento.Ingresso{
		SolaLettura: true,
		Clienti:     []censimento.Cliente{{ID: acme, Nome: "ACME", Ragione: "Acme S.p.A.", Attivo: true, Regole: regoleACME}},
		Messaggi: []censimento.Messaggio{{Cliente: acme, Thread: t1, Oggetto: "RDO 400012345 7120001", Corpo: "Richiesta per 7120001",
			Interpretato: true, Estratto: true, Salvati: []string{"7120001", "7120099"}}},
		File: []censimento.File{
			{Cliente: acme, Thread: t1, Nome: "7120001_PRT.stp", Contenuto: "sha-prt1"},
			{Cliente: acme, Thread: t2, Nome: "7120002_PRT.stp", Contenuto: "sha-prt2"},
			{Cliente: acme, Thread: t1, Nome: "7120001A_1.pdf", Contenuto: "sha-a1", Deciso: true, Codice: "7120001", Rev: "1", Proposto: true, CodiceProposto: "7120001", RevProposta: "1"},
		},
		Pezzi: []censimento.Pezzo{
			{Cliente: acme, Thread: t1, Codice: "7120001", Deciso: db.TipoComponenteFinito},
			{Cliente: acme, Thread: t2, Codice: "7120002", Deciso: db.TipoComponenteFinito},
			{Cliente: acme, Thread: t1, Codice: "7120010", Nomi: []string{"VITE TE M8X20 UNI 5739"}, Deciso: db.TipoComponenteCommerciale},
		},
		DominiNonCensiti: []censimento.Voce{{Testo: "fornitore-esempio.example", N: 2}},
	}
	esegui := func(c censimento.Censimento) string {
		t.Helper()
		var buf bytes.Buffer
		if err := s.pagine["forme.html"].ExecuteTemplate(&buf, "contenuto", vista{Dati: vistaForme(c, time.Now())}); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}

	html := esegui(censimento.Aggrega(in))
	for _, atteso := range []string{
		"Forme viste", `href="/admin/anagrafica/forme.md"`, "transazione di sola lettura: sì",
		"fornitore-esempio.example", `href="#cliente-` + acme.String() + `"`, `id="cliente-` + acme.String() + `"`,
		"<th>mail</th><th>nome</th><th>step</th><th>cartiglio</th><th>distinta</th>",
		"9999999_AAA", "_PRT", "<b>sì</b>", "7120001A rev 1", "7120001 rev 1",
		"Pezzi decisi: 1 particolari commerciali, 2 altri", "<td>VITE</td>",
		"arriva con la fase 4.4a.1", "Particolari commerciali decisi senza una proposta",
		"RDO 999999999 9999999",
		"sulle 1 mail lette dall'ingest con un'estrazione salvata: codici salvati allora 2", "salvati allora e non trovati oggi <b>1</b>", "(7120099)",
		"Lette con «ignora» e senza codici salvati, fuori dal confronto: 0",
		"3 nomi di file tecnici (uno per RFQ: la lettura dipende solo dal nome)", "un codice: 1 file (ogni contenuto con la sua decisione)",
		"File con una proposta di file salvata: 1", "<th>Proposta salvata</th>",
	} {
		if !strings.Contains(html, atteso) {
			t.Errorf("manca %q nella pagina", atteso)
		}
	}
	for _, vietato := range []string{"<form", "hx-post", `method="post"`} {
		if strings.Contains(html, vietato) {
			t.Errorf("la pagina delle forme non manda niente: c'e' %q", vietato)
		}
	}

	// con una proposta scartata (le proposte arrivano con la 4.4a.1): la riga con i due tipi e il motivo
	in.Pezzi[2].Proposto, in.Pezzi[2].Deciso, in.Pezzi[2].Motivo = db.TipoComponenteCommerciale, db.TipoComponenteSciolto, "normato: UNI 5739"
	html = esegui(censimento.Aggrega(in))
	for _, atteso := range []string{"scartate dall'ingegnere: <b>1</b>", "Proposte scartate dall&#39;ingegnere", "<td>particolare</td><td>particolare commerciale</td><td>normato: UNI 5739</td>"} {
		if !strings.Contains(html, atteso) {
			t.Errorf("con la proposta scartata manca %q", atteso)
		}
	}
}
