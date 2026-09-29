//go:build integrazione

// L4 — Smistamento 4.13b: le voci della posta del cliente sull'ingest vero. Un mittente di sistema salva
// l'evento dichiarato (l'atto), nessuna RFQ nuova proposta, nessun identificativo, e i codici della mail
// fra i candidati con il ruolo «non classificato»; il numero d'ordine nella forma del cliente non è un
// candidato di codice né il codice del file che lo porta nel nome, e la mail è un ordine; con il numero
// d'ordine anticipato un ordine accanto a una richiesta si salva come richiesta d'offerta. La controprova è
// la stessa mail dal buyer, e dallo stesso cliente senza le voci. Cliente, indirizzi e numeri inventati.
//
// Correzione 1 della 4.13b: il numero d'ordine non va nemmeno fra i codici citati da un nome che non è un
// codice («Ordine ODA_0001234.pdf» → `codici_nel_nome`, che il Fascicolo rilegge come codice); e con il numero
// d'ordine anticipato un sollecito con l'ordine e la parola «RDO» si salva come sollecito, non come richiesta.

package ingest

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
)

const regoleVociPosta = `{
  "famiglie_codice": [{"regex": "\\b7120\\d{3}\\b", "descrizione": "codici ACME", "esempio": "7120001"}],
  "numero_ordine_anticipato": true,
  "numero_ordine": {"regex": "\\bODA_\\d{7}\\b", "descrizione": "ordini ODA", "esempio": "ODA_0001234"},
  "mittenti_sistema": [
    {"mittente": "avvisi@acme.example", "evento": "ALTRO", "descrizione": "avvisi del gestionale", "esempio": "avvisi@acme.example"},
    {"mittente": "ordini@acme.example", "evento": "ORDINE", "esempio": "ordini@acme.example"}
  ]
}`

// ruoliDeiCandidati: "codice/ruolo" dei candidati di codice del messaggio, in ordine.
func (b *bancoControparte) ruoliDeiCandidati(messaggio uuid.UUID) string {
	b.t.Helper()
	var s string
	if err := b.pool.QueryRow(b.ctx, `SELECT coalesce(string_agg(codice || '/' || ruolo::text, ' ' ORDER BY codice), '')
		FROM candidato_codice WHERE messaggio_id = $1`, messaggio).Scan(&s); err != nil {
		b.t.Fatal(err)
	}
	return s
}

func TestLeVociDellaPostaNellIngest(t *testing.T) {
	b := nuovoBancoControparte(t)
	b.clienteConRegole("ACME", regoleVociPosta, "acme.example")
	b.clienteConRegole("BETA", `{"famiglie_codice": [{"regex": "\\b7120\\d{3}\\b", "esempio": "7120001"}]}`, "beta.example")

	const oggetto = "RFQ 7120001 - richiesta d'offerta"
	const corpo = "Nuova RFQ disponibile per i codici 7120001 e 7120010."
	casi := []struct {
		chiave, da            string
		atto                  string
		esito                 db.EsitoTriage
		identificativi, ruoli string
		testaMotivi, perche   string
	}{
		{"<413b-avviso@acme.example>", "avvisi@acme.example", classificazione.AttoNotifica, db.EsitoTriageIgnora, "",
			"7120001/non_classificato 7120010/non_classificato", "evento · ALTRO (chiaro): mittente di sistema del cliente", "un avviso non apre una RFQ"},
		{"<413b-ordine@acme.example>", "ordini@acme.example", classificazione.AttoOrdine, db.EsitoTriageIgnora, "",
			"7120001/non_classificato 7120010/non_classificato", "evento · ORDINE (chiaro): mittente di sistema del cliente", "un ordine di sistema non apre una RFQ"},
		// la controprova: la stessa mail dal buyer è una richiesta nuova, con i codici proposti
		{"<413b-buyer@acme.example>", "buyer@acme.example", classificazione.AttoRichiestaOfferta, db.EsitoTriageNuovaRfq, "7120001 7120010",
			"7120001/prodotto 7120010/prodotto", "evento · NUOVA_RFQ", "dal buyer"},
	}
	for _, c := range casi {
		b.ingerisci(b.dalCliente(c.chiave, "CONV-"+c.chiave, "", c.da, oggetto, corpo))
		b.nessunLegame(c.chiave, "le voci della posta non agganciano")
		m := b.messaggio(c.chiave)
		p, ok := b.proposta(m.MessaggioID)
		if !ok {
			t.Fatalf("%s: nessuna proposta", c.perche)
		}
		if p.Atto.String != c.atto || p.Esito != c.esito {
			t.Errorf("%s: atto %q, esito %s; attesi %q, %s", c.perche, p.Atto.String, p.Esito, c.atto, c.esito)
		}
		if got := strings.Join(slices.Sorted(slices.Values(p.Identificativi)), " "); got != c.identificativi {
			t.Errorf("%s: identificativi %q, attesi %q", c.perche, got, c.identificativi)
		}
		if got := b.ruoliDeiCandidati(m.MessaggioID); got != c.ruoli {
			t.Errorf("%s: candidati di codice %q, attesi %q", c.perche, got, c.ruoli)
		}
		var motivi []string
		_ = json.Unmarshal(p.Motivi, &motivi)
		if len(motivi) == 0 || !strings.HasPrefix(motivi[0], c.testaMotivi) {
			t.Errorf("%s: i motivi non cominciano con %q: %q", c.perche, c.testaMotivi, motivi)
		}
	}

	// il numero d'ordine del cliente: non è un candidato di codice, non è il codice del file che lo porta nel
	// nome, e la mail è un ordine
	oda := b.dalCliente("<413b-oda@acme.example>", "CONV-413b-oda", "", "buyer@acme.example", "ODA_0001234", "In allegato il documento per 20 pezzi del 7120010.")
	oda.Allegati = allegatiConOrdine()
	b.ingerisci(oda)
	m := b.messaggio("<413b-oda@acme.example>")
	if got := b.candidatiDiCodice(m.MessaggioID); got != "7120010/-" {
		t.Errorf("il numero d'ordine fra i candidati di codice: %q", got)
	}
	// il nome che è l'ordine non ha un codice; il nome che cita l'ordine non lo cita come codice (correzione 1),
	// e il codice vero accanto all'ordine resta citato
	for nome, atteso := range map[string]string{
		"ODA_0001234.pdf":                    "-/-",
		"Ordine ODA_0001234.pdf":             "-/-",
		"Ordine ODA_0001234 per 7120010.pdf": `-/- ["7120010"]`,
	} {
		if got := b.proposteDegliAllegati(m.MessaggioID)[nome]; got != atteso {
			t.Errorf("il nome «%s»: proposta %q, attesa %q", nome, got, atteso)
		}
	}
	if p, _ := b.proposta(m.MessaggioID); p.Atto.String != classificazione.AttoOrdine {
		t.Errorf("la mail con il numero d'ordine: atto %q", p.Atto.String)
	}

	// il numero d'ordine anticipato: un ordine accanto a una richiesta si salva come richiesta d'offerta
	b.ingerisci(b.dalCliente("<413b-anticipato@acme.example>", "CONV-413b-ant", "", "buyer@acme.example", "Codice 7120001",
		"Buongiorno, richiesta d'offerta per il 7120001: il nostro ordine n. 4500012345 è già emesso."))
	if p, _ := b.proposta(b.messaggio("<413b-anticipato@acme.example>").MessaggioID); p.Atto.String != classificazione.AttoRichiestaOfferta {
		t.Errorf("l'ordine anticipato: atto %q", p.Atto.String)
	}
	// correzione 1: il sollecito con l'ordine e la parola «RDO» resta un sollecito
	b.ingerisci(b.dalCliente("<413b-sollecito@acme.example>", "CONV-413b-sol", "", "buyer@acme.example", "Sollecito RDO 12345",
		"Vi sollecitiamo l'offerta, nostro ordine n. 4500012345."))
	if p, _ := b.proposta(b.messaggio("<413b-sollecito@acme.example>").MessaggioID); p.Atto.String != classificazione.AttoSollecito {
		t.Errorf("il sollecito con l'ordine anticipato: atto %q", p.Atto.String)
	}

	// la controprova dello stesso gruppo di mail per un cliente senza le voci: l'avviso è una RFQ nuova, il
	// numero d'ordine è un codice generico, l'ordine con la richiesta è un ordine
	b.ingerisci(b.dalCliente("<413b-avviso@beta.example>", "CONV-413b-b1", "", "avvisi@beta.example", oggetto, corpo))
	if p, _ := b.proposta(b.messaggio("<413b-avviso@beta.example>").MessaggioID); p.Esito != db.EsitoTriageNuovaRfq || p.Atto.String != classificazione.AttoRichiestaOfferta {
		t.Errorf("senza le voci, l'avviso: %s/%q", p.Esito, p.Atto.String)
	}
	odaB := b.dalCliente("<413b-oda@beta.example>", "CONV-413b-b2", "", "buyer@beta.example", "ODA_0001234", "In allegato il documento per 20 pezzi del 7120010.")
	odaB.Allegati = allegatiConOrdine()
	b.ingerisci(odaB)
	mB := b.messaggio("<413b-oda@beta.example>")
	if got := b.candidatiDiCodice(mB.MessaggioID); got != "7120010/- ODA_0001234/-" {
		t.Errorf("senza le voci, i candidati: %q", got)
	}
	for nome, atteso := range map[string]string{
		"ODA_0001234.pdf":                    "ODA_0001234/-",
		"Ordine ODA_0001234.pdf":             `-/- ["ODA_0001234"]`,
		"Ordine ODA_0001234 per 7120010.pdf": `-/- ["ODA_0001234", "7120010"]`,
	} {
		if got := b.proposteDegliAllegati(mB.MessaggioID)[nome]; got != atteso {
			t.Errorf("senza le voci, il nome «%s»: proposta %q, attesa %q", nome, got, atteso)
		}
	}
	b.ingerisci(b.dalCliente("<413b-anticipato@beta.example>", "CONV-413b-b3", "", "buyer@beta.example", "Codice 7120001",
		"Buongiorno, richiesta d'offerta per il 7120001: il nostro ordine n. 4500012345 è già emesso."))
	if p, _ := b.proposta(b.messaggio("<413b-anticipato@beta.example>").MessaggioID); p.Atto.String != classificazione.AttoOrdine {
		t.Errorf("senza le voci, l'ordine con la richiesta: atto %q", p.Atto.String)
	}
	b.ingerisci(b.dalCliente("<413b-sollecito@beta.example>", "CONV-413b-b4", "", "buyer@beta.example", "Sollecito RDO 12345",
		"Vi sollecitiamo l'offerta, nostro ordine n. 4500012345."))
	if p, _ := b.proposta(b.messaggio("<413b-sollecito@beta.example>").MessaggioID); p.Atto.String != classificazione.AttoOrdine {
		t.Errorf("senza le voci, il sollecito con l'ordine: atto %q", p.Atto.String)
	}
}

// allegatiConOrdine: un nome che è il numero d'ordine, uno che lo cita, e uno che cita l'ordine e un codice vero.
func allegatiConOrdine() []worker.AllegatoIn {
	return []worker.AllegatoIn{
		{Indice: 1, NomeFile: "ODA_0001234.pdf", Estensione: "pdf", Natura: "file", Bytes: 1000},
		{Indice: 2, NomeFile: "Ordine ODA_0001234.pdf", Estensione: "pdf", Natura: "file", Bytes: 1000},
		{Indice: 3, NomeFile: "Ordine ODA_0001234 per 7120010.pdf", Estensione: "pdf", Natura: "file", Bytes: 1000},
	}
}
