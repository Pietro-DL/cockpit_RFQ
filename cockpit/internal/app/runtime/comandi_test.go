package runtime

// L1 — i pezzi puri del comando U5 (Smistamento F7, addendum A5.15.2, P38): il controllo del database appena
// collegato e la stampa del rapporto. La parte contro PostgreSQL sta in riapertura_db_test.go.

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/fascicolo"
)

// Prova 201 (A5.15.2, P38), dopo la connessione: in applicazione il server deve dire di essere il database
// scritto sulla riga di comando (SELECT current_database()); in anteprima il pool deve essere quello di
// ApriDatabaseInLettura (default_transaction_read_only = on). La seconda riga stampata dice che cosa si e'
// controllato; un rifiuto non stampa niente e non legge altro.
func TestIlComandoControllaIlDatabaseCollegato(t *testing.T) {
	for _, c := range []struct {
		nome        string
		argomento   string
		risposte    map[string]string
		errLettura  error
		frase       string // contenuta nell'errore; "" = passa
		riga        string // la riga stampata quando passa
		domandaSola string // l'unica domanda fatta al database
	}{
		{nome: "applicazione, il server e' quello", argomento: "cockpit_prova_test",
			risposte: map[string]string{"SELECT current_database()": "cockpit_prova_test"},
			riga:     "collegato a cockpit_prova_test (SELECT current_database())\n", domandaSola: "SELECT current_database()"},
		{nome: "applicazione, il server e' un altro", argomento: "cockpit_prova_test",
			risposte: map[string]string{"SELECT current_database()": "cockpit_altro_test"},
			frase:    `il server collegato dice di essere "cockpit_altro_test"`, domandaSola: "SELECT current_database()"},
		{nome: "applicazione, la domanda fallisce", argomento: "cockpit_prova_test", errLettura: errors.New("rotto"),
			frase: "rotto", domandaSola: "SELECT current_database()"},
		{nome: "anteprima su un pool in sola lettura",
			risposte: map[string]string{"SHOW default_transaction_read_only": "on"},
			riga:     "collegato in sola lettura (default_transaction_read_only=on)\n", domandaSola: "SHOW default_transaction_read_only"},
		{nome: "anteprima su un pool scrivibile",
			risposte: map[string]string{"SHOW default_transaction_read_only": "off"},
			frase:    "non ha aperto il database in sola lettura (default_transaction_read_only=off)", domandaSola: "SHOW default_transaction_read_only"},
		{nome: "anteprima, la domanda fallisce", errLettura: errors.New("rotto"), frase: "rotto", domandaSola: "SHOW default_transaction_read_only"},
	} {
		var domande []string
		leggi := func(_ context.Context, sql string) (string, error) {
			domande = append(domande, sql)
			if c.errLettura != nil {
				return "", c.errLettura
			}
			return c.risposte[sql], nil
		}
		var b bytes.Buffer
		err := controllaIlCollegamento(context.Background(), &b, c.argomento, "cockpit_prova_test", leggi)
		switch {
		case c.frase == "" && err != nil:
			t.Errorf("%s: %v", c.nome, err)
		case c.frase != "" && (err == nil || !strings.Contains(err.Error(), c.frase)):
			t.Errorf("%s: %v, atteso che dica %q", c.nome, err, c.frase)
		}
		if c.frase != "" && b.Len() != 0 {
			t.Errorf("%s: dopo un rifiuto ha stampato %q", c.nome, b.String())
		}
		if c.frase == "" && b.String() != c.riga {
			t.Errorf("%s: ha stampato %q, atteso %q", c.nome, b.String(), c.riga)
		}
		if len(domande) != 1 || domande[0] != c.domandaSola {
			t.Errorf("%s: domande al database %q, attesa solo %q", c.nome, domande, c.domandaSola)
		}
	}
}

// A5.15.2 (Domanda 6 = A): la stampa dice le RFQ in revisione a parte. Le RFQ normali vengono prima, poi
// una sola intestazione e le RFQ in revisione, anche se nel rapporto sono mescolate; una RFQ senza niente da
// dire non si stampa; poi i totali, una voce per riga, `chiave=valore`, compreso rfq_in_revisione.
func TestLaStampaDiceLeRfqInRevisioneAParte(t *testing.T) {
	normale, revisione, vuota := uuid.New(), uuid.New(), uuid.New()
	conUnAggancio := fascicolo.FormeLegacy{Agganci: []fascicolo.NodoAgganciato{{}},
		Effetto: fascicolo.EffettoNelGate{Frase: "il gate si fermerà su 1 figlio diretto da confermare in 1 file autorizzato"}}
	r := fascicolo.RapportoRiapertura{Modalita: fascicolo.ModalitaApplicazione, Avvisi: []string{"dopo il backup"},
		Rfq: []fascicolo.RfqRiapertura{
			{ThreadID: revisione, Cartella: `ACME\WIP\rev`, Versione: "V2 bozza", InRevisione: true, Forme: conUnAggancio,
				Esito: fascicolo.EsitoRiaperta, NodiRiaperti: 1},
			{ThreadID: vuota, Cartella: `ACME\WIP\vuota`, Versione: "working", Esito: fascicolo.EsitoNiente},
			{ThreadID: normale, Cartella: `ACME\WIP\norm`, Versione: "working", Forme: conUnAggancio,
				Esito: fascicolo.EsitoRiaperta, NodiRiaperti: 1},
		},
		Totali: fascicolo.TotaliRiapertura{NelPerimetro: 3, InRevisione: 1, EscluseCongelate: 2, Agganci: 2, Riaperte: 2, NodiRiaperti: 2}}
	var b bytes.Buffer
	StampaRiapertura(&b, r)
	out := b.String()
	intestazione := "RFQ in revisione (si riapre solo la bozza; le versioni congelate e le istantanee non si toccano):\n"
	if n := strings.Count(out, intestazione); n != 1 {
		t.Fatalf("l'intestazione delle RFQ in revisione compare %d volte:\n%s", n, out)
	}
	iNorm, iInt, iRev := strings.Index(out, "rfq="+normale.String()), strings.Index(out, intestazione), strings.Index(out, "rfq="+revisione.String())
	if iNorm < 0 || iRev < 0 || !(iNorm < iInt && iInt < iRev) {
		t.Errorf("ordine: normale %d, intestazione %d, in revisione %d (attesi in quest'ordine):\n%s", iNorm, iInt, iRev, out)
	}
	if strings.Contains(out, vuota.String()) {
		t.Errorf("una RFQ senza niente da dire si stampa:\n%s", out)
	}
	if !strings.HasPrefix(out, "modalita=applicazione\n") || strings.Contains(out, "sola_lettura=") {
		t.Errorf("la testa della stampa:\n%s", out)
	}
	for _, riga := range []string{"rfq_nel_perimetro=3", "rfq_in_revisione=1", "rfq_escluse_congelate=2", "fa_nodi=2",
		"rfq_riaperte=2", "nodi_riaperti=2", "archi_riaperti=0", "rfq_fallite=0", "avviso: dopo il backup"} {
		if !strings.Contains(out, "\n"+riga+"\n") {
			t.Errorf("manca la riga %q:\n%s", riga, out)
		}
	}
	// i totali vengono dopo l'ultima RFQ
	if i := strings.Index(out, "\nrfq_nel_perimetro="); i < iRev {
		t.Errorf("i totali prima delle RFQ:\n%s", out)
	}
}
