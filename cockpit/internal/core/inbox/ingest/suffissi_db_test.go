//go:build integrazione

// L4 — Fascicolo v3: i suffissi decorativi del cliente nell'ingest. Un allegato «77720000_PRT.pdf» di un
// cliente che dichiara «_PRT» e' il disegno del pezzo 77720000: la proposta dal nome, i codici trovati in un
// nome che non e' un codice, i codici citati per il portale e i candidati di codice del messaggio lo dicono
// senza la coda. Per un fornitore valgono i suffissi dei clienti che gli hanno mandato richieste. Per chi non
// li dichiara non cambia niente. La regola pura sta in classificazione (suffissi_test.go).

package ingest

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
)

const regoleConSuffissoPRT = `{"suffissi_decorativi": ["_PRT"]}`

// clienteConRegole censisce un cliente con il suo dominio e le sue regole.
func (b *bancoControparte) clienteConRegole(cartella, regole string, domini ...string) db.Cliente {
	b.t.Helper()
	c := b.cliente(cartella, domini...)
	c, err := b.q.SetRegoleCliente(b.ctx, db.SetRegoleClienteParams{ClienteID: c.ClienteID, Regole: json.RawMessage(regole)})
	if err != nil {
		b.t.Fatal(err)
	}
	return c
}

// allegatiConSuffisso sono gli allegati della prova: due disegni con il suffisso (uno con la revisione prima
// del suffisso) e un nome che non e' un codice ma ne contiene uno.
func allegatiConSuffisso() []worker.AllegatoIn {
	return []worker.AllegatoIn{
		{Indice: 1, NomeFile: "77720000_PRT.pdf", Estensione: "pdf", Natura: "file", Bytes: 1000},
		{Indice: 2, NomeFile: "77730000_C_PRT.pdf", Estensione: "pdf", Natura: "file", Bytes: 1000},
		{Indice: 3, NomeFile: "Offerta 77840000_PRT per staffe.pdf", Estensione: "pdf", Natura: "file", Bytes: 1000},
	}
}

const corpoConPortale = "Buongiorno, richiesta d'offerta per i particolari in allegato.\nVi abbiamo caricato sul portale il CAD del codice 77722757_PRT."

// proposteDegliAllegati: nome del file → "codice/rev", con i codici trovati nel nome se ci sono.
func (b *bancoControparte) proposteDegliAllegati(messaggio uuid.UUID) map[string]string {
	b.t.Helper()
	righe, err := b.pool.Query(b.ctx, `SELECT a.nome_file, coalesce(d.codice, '-') || '/' || coalesce(d.rev, '-') ||
		coalesce(' ' || (d.dettagli -> 'codici_nel_nome')::text, '')
		FROM documento_proposta d JOIN allegato a USING (allegato_id) WHERE a.messaggio_id = $1`, messaggio)
	if err != nil {
		b.t.Fatal(err)
	}
	defer righe.Close()
	out := map[string]string{}
	for righe.Next() {
		var nome, v string
		if err := righe.Scan(&nome, &v); err != nil {
			b.t.Fatal(err)
		}
		out[nome] = v
	}
	if err := righe.Err(); err != nil {
		b.t.Fatal(err)
	}
	return out
}

func (b *bancoControparte) codiciDelPortale(messaggio uuid.UUID) string {
	b.t.Helper()
	var s string
	if err := b.pool.QueryRow(b.ctx, `SELECT coalesce(string_agg(coalesce(codice, '-'), ' ' ORDER BY codice), '')
		FROM riferimento_portale WHERE messaggio_id = $1`, messaggio).Scan(&s); err != nil {
		b.t.Fatal(err)
	}
	return s
}

// candidatiDiCodice: "codice/rev" dei candidati di codice del messaggio, in ordine.
func (b *bancoControparte) candidatiDiCodice(messaggio uuid.UUID) string {
	b.t.Helper()
	var s string
	if err := b.pool.QueryRow(b.ctx, `SELECT coalesce(string_agg(codice || '/' || coalesce(nullif(rev, ''), '-'), ' ' ORDER BY codice), '')
		FROM candidato_codice WHERE messaggio_id = $1`, messaggio).Scan(&s); err != nil {
		b.t.Fatal(err)
	}
	return s
}

func confronta(t *testing.T, chi string, got, want map[string]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s: %d proposte %v, attese %d %v", chi, len(got), got, len(want), want)
	}
	for nome, w := range want {
		if g, ok := got[nome]; !ok || g != w {
			t.Errorf("%s, %s: proposta %q, attesa %q", chi, nome, g, w)
		}
	}
}

// La mail di un cliente che dichiara «_PRT»: gli allegati si propongono con il codice del pezzo (e la
// revisione riletta dalla coda), i codici dentro un nome che non e' un codice e quelli citati per il portale
// perdono il suffisso, e i candidati di codice del messaggio non hanno la coda. La stessa mail di un cliente
// che non dichiara suffissi e' il controllo: tutto resta come lo legge l'estrattore.
func TestLIngestTogliIlSuffissoDecorativoSoloAlClienteCheLoDichiara(t *testing.T) {
	b := nuovoBancoControparte(t)
	b.clienteConRegole("ACME", regoleConSuffissoPRT, "acme.example")
	b.cliente("BETA", "beta.example")

	daAcme := b.richiestaDOfferta("acquisti@acme.example")
	daAcme.CorpoTesto, daAcme.Allegati = corpoConPortale, allegatiConSuffisso()
	daBeta := b.richiestaDOfferta("acquisti@beta.example")
	daBeta.CorpoTesto, daBeta.Allegati = corpoConPortale, allegatiConSuffisso()
	b.ingerisci(daAcme, daBeta)

	acme, beta := b.messaggio(daAcme.MessageID), b.messaggio(daBeta.MessageID)
	if acme.ControparteTipo != db.TipoControparteCliente || beta.ControparteTipo != db.TipoControparteCliente {
		t.Fatalf("le due mail vengono da due clienti censiti, altrimenti la prova non prova niente: %s, %s", acme.ControparteTipo, beta.ControparteTipo)
	}

	confronta(t, "ACME", b.proposteDegliAllegati(acme.MessaggioID), map[string]string{
		"77720000_PRT.pdf":                    "77720000/-",
		"77730000_C_PRT.pdf":                  "77730000/C",
		"Offerta 77840000_PRT per staffe.pdf": `-/- ["77840000"]`,
	})
	confronta(t, "Beta", b.proposteDegliAllegati(beta.MessaggioID), map[string]string{
		"77720000_PRT.pdf":                    "77720000_PRT/-",
		"77730000_C_PRT.pdf":                  "77730000_C_PRT/-",
		"Offerta 77840000_PRT per staffe.pdf": `-/- ["77840000_PRT"]`,
	})

	if got := b.codiciDelPortale(acme.MessaggioID); got != "77722757" {
		t.Errorf("ACME: codici citati per il portale %q, atteso 77722757", got)
	}
	if got := b.codiciDelPortale(beta.MessaggioID); got != "77722757_PRT" {
		t.Errorf("Beta: codici citati per il portale %q, atteso 77722757_PRT", got)
	}

	// i candidati di codice leggono oggetto, corpo e nomi degli allegati con lo stesso motore
	if got, want := b.candidatiDiCodice(acme.MessaggioID), "77720000/- 77722757/- 77730000/C 77840000/-"; got != want {
		t.Errorf("ACME: candidati di codice %q, attesi %q", got, want)
	}
	if got, want := b.candidatiDiCodice(beta.MessaggioID), "77720000_PRT/- 77722757_PRT/- 77730000_C_PRT/- 77840000_PRT/-"; got != want {
		t.Errorf("Beta: candidati di codice %q, attesi %q", got, want)
	}
}

// famigliaConCoda e' una famiglia che prende anche la coda del nome («77720000_PRT»): per la posta di un
// fornitore contano solo i codici di famiglia, e la famiglia dice gia' dov'e' il codice. Con il suffisso
// dichiarato si toglie solo quello.
const (
	famigliaConCoda = `{"famiglie_codice": [{"regex": "\\b(?P<codice>777\\d{5}(?:_[A-Z]{3})?)", "descrizione": "disegni 777",
		"esempio": "77720000_PRT"}]`
	famigliaConCodaESuffisso     = famigliaConCoda + `, "suffissi_decorativi": ["_PRT"]}`
	famigliaConCodaSenzaSuffissi = famigliaConCoda + `}`
)

// La posta di un fornitore si legge con le regole dei clienti che gli hanno mandato richieste (PerFornitore),
// suffissi compresi: il disegno che il fornitore rimanda con il nome del CAD di ACME e' il pezzo di ACME, nella
// proposta dal nome e nei candidati di codice. Un fornitore che ha richieste solo da un cliente con la stessa
// famiglia ma senza suffissi e' il controllo.
func TestLaPostaDelFornitoreUsaISuffissiDeiClientiCheGliHannoChiesto(t *testing.T) {
	b := nuovoBancoControparte(t)
	acme := b.clienteConRegole("ACME", famigliaConCodaESuffisso, "acme.example")
	beta := b.clienteConRegole("BETA", famigliaConCodaSenzaSuffissi, "beta.example")
	minuterie := b.fornitore("Minuterie Esempio di prova", db.TipoFornitoreProcessi, "minuterie-esempio.example")
	torneria := b.fornitore("Torneria Beta di prova", db.TipoFornitoreProcessi, "torneria.example")
	b.richiesta(b.rfq(acme, "RFQ ACME 77720000", "77720000"), minuterie, db.StatoRichiestaFornitoreInviata, uuid.NullUUID{}, "")
	b.richiesta(b.rfq(beta, "RFQ Beta 77720000", "77720000"), torneria, db.StatoRichiestaFornitoreInviata, uuid.NullUUID{}, "")

	allegato := []worker.AllegatoIn{{Indice: 1, NomeFile: "77720000_PRT.pdf", Estensione: "pdf", Natura: "file", Bytes: 1000}}
	daMinuterie := b.rispostaDelFornitore("<suff-minuterie@minuterie-esempio.example>", "CONV-SUFF-MIN", "info@minuterie-esempio.example", "Nostra offerta", "In allegato il disegno quotato.")
	daMinuterie.Allegati = allegato
	daTorneria := b.rispostaDelFornitore("<suff-torneria@torneria.example>", "CONV-SUFF-TOR", "info@torneria.example", "Nostra offerta", "In allegato il disegno quotato.")
	daTorneria.Allegati = allegato
	b.ingerisci(daMinuterie, daTorneria)

	m, tor := b.messaggio(daMinuterie.MessageID), b.messaggio(daTorneria.MessageID)
	if m.ControparteTipo != db.TipoControparteFornitore || tor.ControparteTipo != db.TipoControparteFornitore {
		t.Fatalf("le due mail vengono da due fornitori censiti: %s, %s", m.ControparteTipo, tor.ControparteTipo)
	}
	confronta(t, "Minuterie Esempio (richieste da ACME)", b.proposteDegliAllegati(m.MessaggioID), map[string]string{"77720000_PRT.pdf": "77720000/-"})
	confronta(t, "Torneria (richieste da Beta)", b.proposteDegliAllegati(tor.MessaggioID), map[string]string{"77720000_PRT.pdf": "77720000_PRT/-"})
	if got := b.candidatiDiCodice(m.MessaggioID); got != "77720000/-" {
		t.Errorf("Minuterie Esempio: candidati di codice %q, atteso 77720000", got)
	}
	if got := b.candidatiDiCodice(tor.MessaggioID); got != "77720000_PRT/-" {
		t.Errorf("Torneria: candidati di codice %q, atteso 77720000_PRT", got)
	}
}
