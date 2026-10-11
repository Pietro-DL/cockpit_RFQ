//go:build integrazione

// L4 — giro 5, A1d-Q1: le anagrafiche prima e dopo l'avvio (direttiva A1d §2.3, bozza §4; Q-A1d-02 = A [U]:
// cockpit_test per le L4 sintetiche). Su uno schema azzerato con le guardie di testutil: il seme dei clienti e quello
// dei fornitori, sintetici (clienti ACME inventati, domini *.example) e passati dalle funzioni vere dei due semi;
// qualche correzione fatta «a mano» con le query dei gestori admin; l'impronta di tutte le tabelle anagrafiche (righe
// ordinate, UUID e cliente.regole compresi, sha256); poi il percorso dell'avvio normale fino a prima di Ascolta
// (ApriDatabase → Semina → ImpostaCapacita → RicalcolaControparti → CostruisciServizi, che chiama preparaMotoreA), due
// volte; poi i due semi una seconda volta; poi [motore_a] accesa con l'indice assente e con l'indice non valido.
// L'impronta non deve cambiare mai: stessi UUID, correzioni a mano conservate, nessuna riga nuova.
//
// Che cosa non prova: il seme reale (i due file privati dei semi) e la forma reale del database del 02/10 (A1d §2.3,
// prova sulla copia TEMPLATE, non autorizzata qui).

package runtime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	risorse "promatec/cockpit"
	"promatec/cockpit/internal/core/inbox/ingest"
	"promatec/cockpit/internal/core/registro/anagrafica"
	"promatec/cockpit/internal/core/registro/fornitori"
	"promatec/cockpit/internal/core/registro/regole"
	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/config"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/testutil"
	motorea_web "promatec/cockpit/internal/transport/web/motore_a"
)

// semeClientiQ1: tre clienti inventati. Due hanno regole legacy minime e valide (passano da regole.ValidaRegole dentro
// anagrafica.Leggi), il terzo nessuna, come nascono tutti.
const semeClientiQ1 = `{"clienti":[
 {"ragione_sociale":"ACME Meccanica S.p.A.","cartella_nas":"ACME-MECCANICA","lingua":"it","peso":12,
  "domini":["acme-meccanica.example"],
  "buyer":[{"cognome":"Rossi","nome":"Mario","email":"Mario.Rossi@acme-meccanica.example","tipo":"buyer"},
           {"cognome":"Bianchi","nome":"Anna","email":"anna.bianchi@acme-meccanica.example","tipo":"buyer"}],
  "regole":{"famiglie_codice":[{"regex":"\\bAC\\d{5}[A-Z]\\b","descrizione":"codici ACME","esempio":"AC12345B"}],
            "richiede_cbd":true}},
 {"ragione_sociale":"ACME Carpenterie S.r.l.","cartella_nas":"ACME-CARPENTERIE","lingua":"en","peso":5,
  "domini":["acme-carpenterie.example","acme-carp.example"],
  "buyer":[{"cognome":"Verdi","email":"l.verdi@acme-carpenterie.example","tipo":"buyer"}],
  "regole":{"famiglie_codice":[{"regex":"\\bCP-\\d{4}\\b","descrizione":"codici carpenterie","esempio":"CP-1234"}]}},
 {"ragione_sociale":"ACME Laminati S.r.l.","cartella_nas":"ACME-LAMINATI","domini":["acme-laminati.example"]}
]}`

// semeFornitoriQ1: tre fornitori inventati. Le qualifiche dipendono dal seme dei clienti (cartella NAS); una nomina un
// cliente che non c'è e resta «non risolta».
const semeFornitoriQ1 = `{"fornitori":[
 {"ragione_sociale":"Verniciature Esempio Q1","tipo":"verniciatore","lingua":"it",
  "domini":["verniciature-q1.example"],
  "contatti":[{"nome":"Ufficio","email":"Info@Verniciature-Q1.example","ruolo":"commerciale"},
              {"nome":"Qualita","email":"qualita@verniciature-q1.example"}],
  "lavorazioni":["cataforesi","verniciatura_polvere"],
  "qualifiche":[{"cliente":"ACME-MECCANICA","lavorazione":"cataforesi"},
                {"cliente":"ACME-CARPENTERIE","lavorazione":"verniciatura_polvere"}]},
 {"ragione_sociale":"Zincature Esempio Q1","tipo":"processi","domini":["zincature-q1.example"],
  "contatti":[{"email":"ordini@zincature-q1.example"}],
  "lavorazioni":["zincatura"],
  "qualifiche":[{"cliente":"ACME-MECCANICA","lavorazione":"zincatura"},
                {"cliente":"ACME-INESISTENTE","lavorazione":"zincatura"}]},
 {"ragione_sociale":"Acciai Esempio Q1","tipo":"materie_prime",
  "contatti":[{"email":"vendite@acciai-q1.example"}]}
]}`

// tabelleAnagraficheQ1: le tabelle dell'audit (01-analisi/2026-10-11-03, par.3). cliente porta con sé regole.
var tabelleAnagraficheQ1 = []string{"cliente", "dominio_cliente", "buyer", "fornitore", "dominio_fornitore",
	"contatto_fornitore", "fornitore_lavorazione", "cliente_fornitore_lavorazione"}

// righeAttese dopo i due semi: 3 clienti, 4 domini, 3 buyer; 3 fornitori, 2 domini, 4 contatti, 3 lavorazioni, 3
// qualifiche (la quarta nomina un cliente che non c'è). Le correzioni a mano aggiungono un contatto.
var righeAtteseQ1 = map[string]int{"cliente": 3, "dominio_cliente": 4, "buyer": 3, "fornitore": 3,
	"dominio_fornitore": 2, "contatto_fornitore": 5, "fornitore_lavorazione": 3, "cliente_fornitore_lavorazione": 3}

type tabellaQ1 struct {
	Righe []string
	Sha   string
}

// improntaQ1: per ogni tabella anagrafica tutte le righe come testo (ogni colonna, UUID e jsonb compresi), in ordine di
// byte, e lo sha256 delle righe unite da «\n». Legge soltanto.
type improntaQ1 map[string]tabellaQ1

func prendiImprontaQ1(t *testing.T, p *pgxpool.Pool) improntaQ1 {
	t.Helper()
	ctx := context.Background()
	imp := improntaQ1{}
	for _, tab := range tabelleAnagraficheQ1 {
		rows, err := p.Query(ctx, `SELECT r::text FROM `+tab+` r ORDER BY r::text COLLATE "C"`)
		if err != nil {
			t.Fatalf("impronta di %s: %v", tab, err)
		}
		var righe []string
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				rows.Close()
				t.Fatalf("impronta di %s: %v", tab, err)
			}
			righe = append(righe, s)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			t.Fatalf("impronta di %s: %v", tab, err)
		}
		somma := sha256.Sum256([]byte(strings.Join(righe, "\n")))
		imp[tab] = tabellaQ1{Righe: righe, Sha: hex.EncodeToString(somma[:])}
	}
	return imp
}

// totale: lo sha256 delle impronte delle tabelle, nell'ordine di tabelleAnagraficheQ1.
func (imp improntaQ1) totale() string {
	h := sha256.New()
	for _, tab := range tabelleAnagraficheQ1 {
		fmt.Fprintf(h, "%s %d %s\n", tab, len(imp[tab].Righe), imp[tab].Sha)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (imp improntaQ1) scrivi(t *testing.T, fase string) {
	t.Helper()
	var parti []string
	for _, tab := range tabelleAnagraficheQ1 {
		parti = append(parti, fmt.Sprintf("%s=%d/%s", tab, len(imp[tab].Righe), imp[tab].Sha[:12]))
	}
	t.Logf("impronta %s: totale %s; %s", fase, imp.totale(), strings.Join(parti, " "))
}

// stessaImprontaQ1: l'impronta dopo deve essere quella di prima, tabella per tabella; se no, le righe che stanno solo
// da una parte (al più cinque per lato).
func stessaImprontaQ1(t *testing.T, fase string, prima, dopo improntaQ1) {
	t.Helper()
	for _, tab := range tabelleAnagraficheQ1 {
		a, b := prima[tab], dopo[tab]
		if a.Sha == b.Sha {
			continue
		}
		t.Errorf("%s: %s è cambiata (%d righe prima, %d dopo)\n  solo prima: %s\n  solo dopo: %s", fase, tab,
			len(a.Righe), len(b.Righe), strings.Join(soloInQ1(a.Righe, b.Righe), "\n    "),
			strings.Join(soloInQ1(b.Righe, a.Righe), "\n    "))
	}
}

func soloInQ1(a, b []string) []string {
	in := map[string]bool{}
	for _, s := range b {
		in[s] = true
	}
	var out []string
	for _, s := range a {
		if !in[s] && len(out) < 5 {
			out = append(out, s)
		}
	}
	return out
}

// logQ1: un log testuale in memoria che si può scrivere da più goroutine (lo scheduler di CostruisciServizi scrive
// anche dopo che la prova ha letto).
type logQ1 struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *logQ1) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *logQ1) righe(parti ...string) []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []string
	for _, r := range strings.Split(l.buf.String(), "\n") {
		ok := true
		for _, p := range parti {
			ok = ok && strings.Contains(r, p)
		}
		if ok && r != "" {
			out = append(out, r)
		}
	}
	return out
}

// tomlQ1 scrive un cockpit.toml sul database di prova, con staging e radice del NAS in cartelle temporanee, e la
// sezione [motore_a] data (vuota = assente, anteprima spenta).
func tomlQ1(t *testing.T, motoreA string) string {
	t.Helper()
	d := t.TempDir()
	corpo := "[db]\ndsn = '" + testutil.DSN(t) + "'\n[nas]\nstaging = '" + filepath.Join(d, "staging") +
		"'\nradice = '" + filepath.Join(d, "nas") + "'\n" + motoreA
	p := filepath.Join(d, "cockpit.toml")
	if err := os.WriteFile(p, []byte(corpo), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// avvioQ1 è l'avvio normale di Esegui fino a prima di PreparaTLS e Ascolta, sul file dato, come lo fa cockpit.exe: la
// configurazione letta dal file, un pool nuovo (un riavvio), le migrazioni, i semi delle fondazioni, le capacità, il
// ricalcolo delle controparti e i servizi, con preparaMotoreA dentro. Poi chiude come chiude un server che si ferma:
// annulla il contesto e chiude il pool. Restituisce le regole del motore A e i messaggi risolti dal ricalcolo.
func avvioQ1(t *testing.T, cfgPath string, log *slog.Logger) (motorea_web.Regole, int) {
	t.Helper()
	cfg, err := config.CaricaConRete(cfgPath, config.Rete{})
	if err != nil {
		t.Fatalf("configurazione: %v", err)
	}
	ctx, annulla := context.WithCancel(context.Background())
	defer annulla()
	pool, versione, err := ApriDatabase(ctx, cfg, risorse.FS, log)
	if err != nil {
		t.Fatalf("ApriDatabase: %v", err)
	}
	defer pool.Close()
	q := db.New(pool)
	if err := Semina(ctx, q, cfg, versione, log); err != nil {
		t.Fatalf("Semina: %v", err)
	}
	capacita, err := ImpostaCapacita(ctx, q, cfg, log)
	if err != nil {
		t.Fatalf("ImpostaCapacita: %v", err)
	}
	n, err := ingest.RicalcolaControparti(ctx, pool, log)
	if err != nil {
		t.Fatalf("RicalcolaControparti: %v", err)
	}
	servizi, err := CostruisciServizi(ctx, cfg, pool, q, log, capacita)
	if err != nil {
		t.Fatalf("CostruisciServizi: %v", err)
	}
	return servizi.MotoreA, n
}

// regoleACMEQ1 copia i tre file di testdata/motore_a (con i CRLF portati a LF, come le prove L1) in una cartella nuova
// e restituisce il percorso dell'indice.
func regoleACMEQ1(t *testing.T) string {
	t.Helper()
	d := filepath.Join(t.TempDir(), "motore_a")
	if err := os.Mkdir(d, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, nome := range []string{"indice_acme.v1.json", "acme.v1.json", "casi_acme.v1.json"} {
		b, err := os.ReadFile(filepath.Join("testdata", "motore_a", nome))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, nome), bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n")), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(d, "indice_acme.v1.json")
}

func tomlMotoreAQ1(indice string) string {
	return "[motore_a]\nanteprima = true\nregole = '" + indice + "'\n"
}

func ptxtQ1(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }

// messaggioQ1: un messaggio in entrata senza controparte (controparte_il NULL), come quelli di prima della 0014. Serve a
// far lavorare davvero RicalcolaControparti, che legge buyer e contatti dei fornitori.
func messaggioQ1(t *testing.T, ctx context.Context, q *db.Queries, chiave, mittente string) uuid.UUID {
	t.Helper()
	quando := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	c, err := q.UpsertConversazione(ctx, db.UpsertConversazioneParams{Canale: db.CanaleOutlook, ChiaveEsterna: "conv-" + chiave,
		PrimoMessaggioIl: quando})
	if err != nil {
		t.Fatal(err)
	}
	m, err := q.UpsertMessaggio(ctx, db.UpsertMessaggioParams{Canale: db.CanaleOutlook, ChiaveEsterna: chiave,
		ConversazioneID: c.ConversazioneID, Direzione: db.DirezioneEntrata, DataEvento: quando,
		MittenteIndirizzo: ptxtQ1(mittente), Destinatari: json.RawMessage(`[]`), Oggetto: ptxtQ1("richiesta di prova")})
	if err != nil {
		t.Fatal(err)
	}
	return m.MessaggioID
}

func TestA1dQ1LeAnagraficheNonCambianoAllAvvioNeAlSecondoLancioDeiSemi(t *testing.T) {
	inizio := time.Now()
	p := testutil.Pool(t)
	ctx := context.Background()
	var database, ruolo string
	if err := p.QueryRow(ctx, "SELECT current_database()::text, current_user::text").Scan(&database, &ruolo); err != nil {
		t.Fatal(err)
	}
	t.Logf("database %s, ruolo %s", database, ruolo)
	testutil.SchemaPulito(t, p) // SchemaVuoto (guardie su nome e ruolo) e tutte le migrazioni
	// ImpostaCapacita cambia le capacità del processo: si rimettono com'erano per le prove che seguono
	t.Cleanup(func() { coda.ImpostaCapacita(tutto) })
	q := db.New(p)

	// 1. I due semi sintetici, con le funzioni vere: anagrafica.Leggi/Semina, fornitori.Leggi/Calcola/Applica.
	semeC, err := anagrafica.Leggi(strings.NewReader(semeClientiQ1))
	if err != nil {
		t.Fatal(err)
	}
	ec, err := anagrafica.Semina(ctx, p, semeC)
	if err != nil {
		t.Fatal(err)
	}
	if len(ec.ClientiCreati) != 3 || ec.DominiAggiunti != 4 || ec.BuyerCreati != 3 || len(ec.Avvisi) != 0 {
		t.Fatalf("primo seme dei clienti: %+v", ec)
	}
	semeF, err := fornitori.Leggi(strings.NewReader(semeFornitoriQ1))
	if err != nil {
		t.Fatal(err)
	}
	ant, err := fornitori.Calcola(ctx, q, semeF)
	if err != nil {
		t.Fatal(err)
	}
	if n := testutil.Conta(t, p, "fornitore"); n != 0 {
		t.Fatalf("l'anteprima dei fornitori ha scritto %d fornitori", n)
	}
	if len(ant.FornitoriDaCreare) != 3 || len(ant.NonRisolti) != 1 || !strings.Contains(ant.NonRisolti[0].String(), "ACME-INESISTENTE") {
		t.Fatalf("anteprima dei fornitori: da creare %v, non risolti %v", ant.FornitoriDaCreare, ant.NonRisolti)
	}
	fatto, err := fornitori.Applica(ctx, p, semeF)
	if err != nil {
		t.Fatal(err)
	}
	if len(fatto.FornitoriDaCreare) != 3 || len(fatto.NonRisolti) != 1 {
		t.Fatalf("applicazione dei fornitori: %+v", fatto)
	}
	dopoSemi := prendiImprontaQ1(t, p)
	dopoSemi.scrivi(t, "dopo i due semi")

	// 2. Due messaggi di prima della 0014, senza controparte: il ricalcolo dell'avvio li risolve leggendo le anagrafiche.
	mCliente := messaggioQ1(t, ctx, q, "q1-cliente", "anna.bianchi@acme-meccanica.example")
	mFornitore := messaggioQ1(t, ctx, q, "q1-fornitore", "qualita@verniciature-q1.example")

	// 3. Correzioni «a mano», con le query dei gestori admin (web/anagrafica.go, anagrafica_admin.go, fornitori_admin.go).
	meccanica, err := q.GetClientePerCartella(ctx, "ACME-MECCANICA")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.UpdateCliente(ctx, db.UpdateClienteParams{ClienteID: meccanica.ClienteID,
		RagioneSociale: "ACME Meccanica Industriale S.p.A.", Profilo: meccanica.Profilo, Lingua: meccanica.Lingua,
		PortaleUrl: meccanica.PortaleUrl, PortaleNote: meccanica.PortaleNote, Peso: 4, Attivo: true,
		Note: ptxtQ1("corretto a mano")}); err != nil {
		t.Fatal(err)
	}
	carpenterie, err := q.GetClientePerCartella(ctx, "ACME-CARPENTERIE")
	if err != nil {
		t.Fatal(err)
	}
	regoleAMano := []byte(`{"famiglie_codice":[{"regex":"\\bCP-\\d{5}\\b","descrizione":"codici carpenterie, corretti a mano","esempio":"CP-12345"}]}`)
	if _, err := regole.ValidaRegole(regoleAMano); err != nil {
		t.Fatalf("le regole corrette a mano non sono valide: %v", err)
	}
	if _, err := q.SetRegoleCliente(ctx, db.SetRegoleClienteParams{ClienteID: carpenterie.ClienteID, Regole: regoleAMano}); err != nil {
		t.Fatal(err)
	}
	rossi, err := q.GetBuyerPerEmail(ctx, "mario.rossi@acme-meccanica.example")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.UpdateBuyer(ctx, db.UpdateBuyerParams{BuyerID: rossi.BuyerID, Cognome: rossi.Cognome, Nome: rossi.Nome,
		Email: rossi.Email, Telefono: ptxtQ1("+39 000 0000000"), Ruolo: ptxtQ1("acquisti, corretto a mano"), Tipo: rossi.Tipo,
		MandaFile: rossi.MandaFile, Lingua: rossi.Lingua, Note: rossi.Note}); err != nil {
		t.Fatal(err)
	}
	zincature, err := q.GetFornitorePerRagioneSociale(ctx, "zincature esempio q1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.UpdateFornitore(ctx, db.UpdateFornitoreParams{FornitoreID: zincature.FornitoreID,
		RagioneSociale: zincature.RagioneSociale, Tipo: zincature.Tipo, Lingua: ptxtQ1("en"),
		Note: ptxtQ1("corretto a mano"), Attivo: true}); err != nil {
		t.Fatal(err)
	}
	verniciature, err := q.GetFornitorePerRagioneSociale(ctx, "verniciature esempio q1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.InsertContattoFornitore(ctx, db.InsertContattoFornitoreParams{FornitoreID: verniciature.FornitoreID,
		Nome: ptxtQ1("Commesse"), Lower: "commesse@verniciature-q1.example", Ruolo: ptxtQ1("aggiunto a mano")}); err != nil {
		t.Fatal(err)
	}

	prima := prendiImprontaQ1(t, p)
	prima.scrivi(t, "prima dell'avvio (semi + correzioni a mano)")
	for tab, n := range righeAtteseQ1 {
		if got := len(prima[tab].Righe); got != n {
			t.Errorf("%s: %d righe, attese %d", tab, got, n)
		}
	}
	correzioniConservate := func(fase string) {
		t.Helper()
		c, err := q.GetClientePerCartella(ctx, "ACME-MECCANICA")
		if err != nil {
			t.Fatal(err)
		}
		if c.ClienteID != meccanica.ClienteID || c.RagioneSociale != "ACME Meccanica Industriale S.p.A." || c.Peso != 4 {
			t.Errorf("%s: ACME-MECCANICA %s %q peso %d: la correzione a mano o l'UUID (%s) non ci sono più", fase,
				c.ClienteID, c.RagioneSociale, c.Peso, meccanica.ClienteID)
		}
		cp, _ := q.GetClientePerCartella(ctx, "ACME-CARPENTERIE")
		if !strings.Contains(string(cp.Regole), "CP-12345") || strings.Contains(string(cp.Regole), "CP-1234\"") {
			t.Errorf("%s: le regole di ACME-CARPENTERIE non sono quelle corrette a mano: %s", fase, cp.Regole)
		}
		b, _ := q.GetBuyerPerEmail(ctx, "mario.rossi@acme-meccanica.example")
		if b.BuyerID != rossi.BuyerID || b.Ruolo.String != "acquisti, corretto a mano" {
			t.Errorf("%s: il buyer Rossi %s ruolo %q: correzione o UUID persi", fase, b.BuyerID, b.Ruolo.String)
		}
		z, _ := q.GetFornitorePerRagioneSociale(ctx, "zincature esempio q1")
		if z.FornitoreID != zincature.FornitoreID || z.Note.String != "corretto a mano" || z.Lingua.String != "en" {
			t.Errorf("%s: Zincature Esempio Q1 %s note %q lingua %q: correzione o UUID persi", fase, z.FornitoreID, z.Note.String, z.Lingua.String)
		}
	}
	correzioniConservate("prima dell'avvio")

	// 4. L'avvio normale, due volte (un riavvio): la prima con [motore_a] assente (spenta), la seconda con l'indice ACME
	// delle prove L1, valido. Il ricalcolo delle controparti scrive i due messaggi la prima volta e nessuno la seconda.
	logA := &logQ1{}
	log := slog.New(slog.NewTextHandler(logA, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ma, n := avvioQ1(t, tomlQ1(t, ""), log)
	if ma.Stato != motorea_web.StatoSpenta || ma.Accesa {
		t.Errorf("primo avvio, [motore_a] assente: stato %s accesa %v, attesa spenta", ma.Stato, ma.Accesa)
	}
	if n != 2 {
		t.Errorf("primo avvio: %d controparti ricalcolate, attese 2", n)
	}
	for id, atteso := range map[uuid.UUID][2]string{
		mCliente:   {string(db.TipoControparteCliente), meccanica.ClienteID.String()},
		mFornitore: {string(db.TipoControparteFornitore), verniciature.FornitoreID.String()},
	} {
		m, err := q.GetMessaggio(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		got := m.ControparteClienteID.UUID.String()
		if m.ControparteTipo == db.TipoControparteFornitore {
			got = m.ControparteFornitoreID.UUID.String()
		}
		if string(m.ControparteTipo) != atteso[0] || got != atteso[1] {
			t.Errorf("messaggio %s: controparte %s %s, attesa %s %s", m.ChiaveEsterna, m.ControparteTipo, got, atteso[0], atteso[1])
		}
	}
	dopo1 := prendiImprontaQ1(t, p)
	dopo1.scrivi(t, "dopo il primo avvio")
	stessaImprontaQ1(t, "primo avvio", prima, dopo1)

	indiceACME := regoleACMEQ1(t)
	ma, n = avvioQ1(t, tomlQ1(t, tomlMotoreAQ1(indiceACME)), log)
	if !ma.Pronta() {
		t.Errorf("secondo avvio, indice ACME valido: stato %s, attesa pronta", ma.Stato)
	}
	if n != 0 {
		t.Errorf("secondo avvio: %d controparti ricalcolate, attese 0", n)
	}
	dopo2 := prendiImprontaQ1(t, p)
	dopo2.scrivi(t, "dopo il secondo avvio")
	stessaImprontaQ1(t, "secondo avvio", prima, dopo2)
	correzioniConservate("dopo i due avvii")

	// 5. I due semi una seconda volta: niente di nuovo, nessun UUID cambiato, le correzioni a mano restano.
	ec2, err := anagrafica.Semina(ctx, p, semeC)
	if err != nil {
		t.Fatal(err)
	}
	if len(ec2.ClientiCreati) != 0 || len(ec2.ClientiPresenti) != 3 || ec2.DominiAggiunti != 0 || ec2.BuyerCreati != 0 || len(ec2.Avvisi) != 0 {
		t.Errorf("secondo seme dei clienti: %+v", ec2)
	}
	ant2, err := fornitori.Calcola(ctx, q, semeF)
	if err != nil {
		t.Fatal(err)
	}
	if !ant2.Vuota() || len(ant2.NonRisolti) != 1 {
		t.Errorf("seconda anteprima dei fornitori: da creare %v, da aggiungere %v, non risolti %v", ant2.FornitoriDaCreare,
			ant2.DaAggiungere, ant2.NonRisolti)
	}
	fatto2, err := fornitori.Applica(ctx, p, semeF)
	if err != nil {
		t.Fatal(err)
	}
	if !fatto2.Vuota() || len(fatto2.FornitoriPresenti) != 3 {
		t.Errorf("seconda applicazione dei fornitori: %+v", fatto2)
	}
	dopoSemi2 := prendiImprontaQ1(t, p)
	dopoSemi2.scrivi(t, "dopo il secondo lancio dei semi")
	stessaImprontaQ1(t, "secondo lancio dei semi", prima, dopoSemi2)
	correzioniConservate("dopo il secondo lancio dei semi")

	// 6. [motore_a] accesa con l'indice assente o non valido: l'avvio va avanti, lo stato dice l'errore, una riga ERROR
	// nel log, e le anagrafiche restano quelle di prima. preparaMotoreA non ha il pool: chiamata da sola non scrive, e
	// l'impronta presa subito dopo lo conferma.
	cartella := filepath.Dir(indiceACME)
	rotto := filepath.Join(cartella, "indice_rotto.v1.json")
	if err := os.WriteFile(rotto, []byte(`{"versione_indice": 1, "limiti": {`), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, v := range []struct {
		nome, indice string
		stato        motorea_web.Stato
	}{
		{"indice assente", filepath.Join(cartella, "non_esiste.v1.json"), motorea_web.StatoIndiceAssente},
		{"indice non valido", rotto, motorea_web.StatoIndiceNonValido},
	} {
		logV := &logQ1{}
		lv := slog.New(slog.NewTextHandler(logV, &slog.HandlerOptions{Level: slog.LevelDebug}))
		diretta := preparaMotoreA(config.MotoreA{Anteprima: true, Regole: v.indice}, lv)
		if !diretta.Accesa || diretta.Stato != v.stato || diretta.Insieme != nil {
			t.Errorf("%s, preparaMotoreA: accesa %v stato %s insieme %v, attesi accesa e %s", v.nome, diretta.Accesa,
				diretta.Stato, diretta.Insieme != nil, v.stato)
		}
		stessaImprontaQ1(t, v.nome+", preparaMotoreA da sola", prima, prendiImprontaQ1(t, p))
		ma, n := avvioQ1(t, tomlQ1(t, tomlMotoreAQ1(v.indice)), lv)
		if !ma.Accesa || ma.Stato != v.stato || ma.Insieme != nil || ma.Pronta() {
			t.Errorf("%s, avvio: accesa %v stato %s, attesi accesa, nessun insieme e %s", v.nome, ma.Accesa, ma.Stato, v.stato)
		}
		if n != 0 {
			t.Errorf("%s, avvio: %d controparti ricalcolate, attese 0", v.nome, n)
		}
		if rr := logV.righe("level=ERROR", "anteprima motore A", "stato="+string(v.stato)); len(rr) < 2 {
			t.Errorf("%s: attese due righe ERROR (chiamata diretta e avvio), trovate %d", v.nome, len(rr))
		}
		dopoV := prendiImprontaQ1(t, p)
		dopoV.scrivi(t, "dopo l'avvio con "+v.nome)
		stessaImprontaQ1(t, "avvio con "+v.nome, prima, dopoV)
	}
	correzioniConservate("alla fine")
	if prima.totale() != prendiImprontaQ1(t, p).totale() {
		t.Error("l'impronta totale finale non è quella di prima dell'avvio")
	}

	// 7. Osservazione, senza asserzioni sul comportamento (non deciso): due correzioni a mano che il seme dei fornitori
	// NON rispetta, viste con la sola anteprima (Calcola, che non scrive). Togliere dall'admin un contatto che viene dal
	// seme: il seme lo rimetterebbe. Rinominare un fornitore: il seme lo cerca per ragione sociale e ne creerebbe uno
	// nuovo col nome vecchio. Si scrive con le query dei gestori admin, poi solo Calcola.
	contatti, err := q.ListContattiFornitore(ctx, zincature.FornitoreID)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range contatti {
		if c.Email == "ordini@zincature-q1.example" {
			if _, err := q.EliminaContattoFornitore(ctx, db.EliminaContattoFornitoreParams{ContattoID: c.ContattoID,
				FornitoreID: zincature.FornitoreID}); err != nil {
				t.Fatal(err)
			}
		}
	}
	acciai, err := q.GetFornitorePerRagioneSociale(ctx, "acciai esempio q1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.UpdateFornitore(ctx, db.UpdateFornitoreParams{FornitoreID: acciai.FornitoreID,
		RagioneSociale: "Acciai Esempio Q1 S.r.l.", Tipo: acciai.Tipo, Lingua: acciai.Lingua, Note: acciai.Note,
		Attivo: true}); err != nil {
		t.Fatal(err)
	}
	primaOss := prendiImprontaQ1(t, p)
	ant3, err := fornitori.Calcola(ctx, q, semeF)
	if err != nil {
		t.Fatal(err)
	}
	stessaImprontaQ1(t, "osservazione: Calcola", primaOss, prendiImprontaQ1(t, p))
	var daAggiungere []string
	for _, r := range ant3.DaAggiungere {
		daAggiungere = append(daAggiungere, r.String())
	}
	sort.Strings(daAggiungere)
	t.Logf("OSSERVAZIONE (seme fornitori dopo correzioni a mano: contatto tolto, fornitore rinominato): da creare %v; "+
		"da aggiungere %v", ant3.FornitoriDaCreare, daAggiungere)
	t.Logf("durata %s", time.Since(inizio).Round(time.Millisecond))
}
