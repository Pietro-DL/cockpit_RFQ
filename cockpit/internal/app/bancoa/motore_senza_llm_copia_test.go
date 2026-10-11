//go:build integrazione && privato

// L4 — MOTORE-SENZA-LLM sulla copia usa e getta del dump (A1c-L4T-01; piano A, 6.7.5 e 3.5; R10, R44, R58 B; scelte
// tecniche T-Q10-06, -07, -08 ratificate): sui thread dell'oracolo, cioè quelli della voce export.thread del manifest
// (FotografiaDaExport, sha256 controllato) che esistono davvero nella copia _run, il percorso del banco (Carica, Calcola,
// Confronta) dà le stesse impronte prima (E1) e dopo (E2) che il ruolo delle copie abbia scritto, per ogni messaggio di
// quei thread, un suggerimento discordante dell'agente, un candidato di codice di origine agente e un triage di fonte
// agente. Il registro SQL non ha testi vietati; le righe scritte si tolgono in t.Cleanup, prima del ricontrollo delle
// sentinelle della copia.
//
// Senza COCKPIT_DATASET_A o COCKPIT_DUMP_COPIA_DSN, con una copia diversa dal manifest, con PGPASSWORD impostata, con
// la _run già sporca di righe dell'agente (va ricreata: la prova non pulisce ciò che non ha scritto), senza gli export o
// con un thread dell'oracolo che la copia non ha: NON ESEGUITA, mai saltata. Gli UUID dei clienti del vecchio export non
// servono e non si usano (D-R115 A).
//
// Nessun valore reale nel codice (E-18): le righe scritte hanno solo valori inventati (ACME, 712xxxx); i numeri vanno
// nel log. La voce del gate «motore senza LLM» resta non eseguita per costruzione: l'esito di questa prova va nel
// registro.

package bancoa

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/core/confronto"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/fotorfq/caricatore"
	valut "promatec/cockpit/internal/core/valutazione"
	"promatec/cockpit/internal/platform/testutil"
)

// I valori inventati dei suggerimenti discordanti dell'agente (ACME, 712xxxx): nessun dato vero si scrive sulla copia.
const (
	q10CodiceDellAgente    = "P7129999"
	q10RisultatoDellAgente = `{"codici": ["7129999", "7120400"], "esito": "aggancia"}`
)

// q10TabellaDellAgente: il nome della tabella dei suggerimenti dell'agente, spezzato come nella guardia
// MOTORE-SENZA-LLM (confronto/motore_senza_llm_db_test.go).
var q10TabellaDellAgente = "analisi" + "_messaggio"

// q10Giro: un giro del percorso del banco sulla copia: la fotografia con la sua impronta, l'esito di valutazione e le
// impronte per thread (confronto, prodotti, ancoraggi) e per allegato (interpretazione), più le diagnostiche per codice.
type q10Giro struct {
	foto            fotorfq.Fotografia
	impronta        string
	esito           valut.Esito
	confronti       map[uuid.UUID]string
	prodotti        map[uuid.UUID]string
	ancoraggi       map[uuid.UUID]string
	interpretazioni map[uuid.UUID]string
	perCodice       []ConteggioCodice
}

// q10GiroCopia: Carica sul pool con il registro (ROLLBACK in fondo, una connessione, nessun testo vietato), poi Calcola
// con gli ingressi del runner e Confronta senza atteso, come l'anteprima; Calcola e Confronta non parlano con il
// database. Un thread o un messaggio chiesti che la copia non ha: NON ESEGUITA (T-Q10-08). I clienti dei casi senza
// thread si chiedono come li chiede il banco (clientiFuori, D-Q10-4).
func q10GiroCopia(t *testing.T, nome string, pr *pgxpool.Pool, reg *testutil.RegistroSQL, thread, fuori []uuid.UUID, b *banco) q10Giro {
	t.Helper()
	f, err := q10CaricaRegistrata(t, nome, pr, reg, caricatore.Richiesta{Thread: thread, Messaggi: fuori, Clienti: b.clientiFuori()})
	if err != nil {
		if strings.Contains(err.Error(), "non esiste") {
			testutil.NonEseguita(t, nome+": un thread dell'oracolo o un messaggio dei casi non c'è nella copia _run (T-Q10-08): "+err.Error())
		}
		t.Fatalf("%s: Carica: %v", nome, err)
	}
	g := q10Giro{foto: f, confronti: map[uuid.UUID]string{}, prodotti: map[uuid.UUID]string{}, ancoraggi: map[uuid.UUID]string{},
		interpretazioni: map[uuid.UUID]string{}}
	if g.impronta, err = fotorfq.ImprontaFotografia(f); err != nil {
		t.Fatalf("%s: impronta della fotografia: %v", nome, err)
	}
	reg.Azzera()
	if g.esito, err = valut.Calcola(f, b.ins, b.ingressi); err != nil {
		t.Fatalf("%s: Calcola: %v", nome, err)
	}
	for _, et := range g.esito.Thread {
		g.confronti[et.ThreadID] = confronto.Confronta(inFiles(et.Confrontabili), inProdotti(et.ProdottiConfrontabili), nil).Impronta
		g.prodotti[et.ThreadID] = et.Prodotti.Impronta
		g.ancoraggi[et.ThreadID] = et.Ancoraggi.Impronta
	}
	for id, fi := range q10FileInterpretati(g.esito) {
		g.interpretazioni[id] = fi.Interpretazione.ID + " " + fi.Interpretazione.Impronta
	}
	g.perCodice = sintesiValutazione(g.esito).PerCodice
	if v := reg.Voci(); len(v) != 0 {
		t.Errorf("%s: Calcola o Confronta hanno parlato con il database (%d testi)", nome, len(v))
	}
	return g
}

// q10Diverse: quante chiavi hanno valori diversi fra due mappe di impronte (o stanno in una sola).
func q10Diverse(a, b map[uuid.UUID]string) int {
	n := 0
	for k, v := range a {
		if w, ok := b[k]; !ok || w != v {
			n++
		}
	}
	for k := range b {
		if _, ok := a[k]; !ok {
			n++
		}
	}
	return n
}

// q10ContaSullaCopia: un conteggio sulla copia _run, per i controlli di pulizia.
func q10ContaSullaCopia(ctx context.Context, p *pgxpool.Pool, sql string) (int, error) {
	var n int
	err := p.QueryRow(ctx, sql).Scan(&n)
	return n, err
}

// q10SqlTriageDellAgente: i triage di fonte agente, che sulla _run pulita non ci sono (A.14b).
const q10SqlTriageDellAgente = `SELECT count(*) FROM proposta_triage WHERE fonte = 'agente'`

func TestL4CopiaMotoreSenzaLLM(t *testing.T) {
	q10SenzaPGPASSWORD(t)
	m := testutil.DatasetA(t)
	p := testutil.PoolCopiaDelDump(t, m.CopiaRun)
	ctx := context.Background()

	// La _run pulita (A.14a, A.14b): righe dell'agente rimaste vogliono dire una copia da ricreare, non un difetto del
	// codice. La prova non le tocca.
	if n := testutil.Conta(t, p, q10TabellaDellAgente); n != 0 {
		testutil.NonEseguita(t, "la copia _run va ricreata: la tabella dei suggerimenti dell'agente non è vuota")
	}
	if n, err := q10ContaSullaCopia(ctx, p, q10SqlTriageDellAgente); err != nil || n != 0 {
		testutil.NonEseguita(t, "la copia _run va ricreata: ci sono triage di fonte agente (o il conteggio non si fa)")
	}

	b := q10IngressiDelRunner(t, testutil.CartellaRapporti(t))
	fe, _ := q10FotografiaDegliExport(t, m)
	var thread []uuid.UUID
	for _, th := range fe.Thread {
		thread = append(thread, th.ID)
	}
	if len(thread) == 0 {
		testutil.NonEseguita(t, "la voce export.thread non ha thread: l'oracolo è vuoto")
	}
	sort.Slice(thread, func(i, j int) bool { return bytes.Compare(thread[i][:], thread[j][:]) < 0 })
	fuori := b.messaggiFuori()
	pr, reg := testutil.PoolConRegistro(t, os.Getenv("COCKPIT_DUMP_COPIA_DSN")) // dopo PoolCopiaDelDump, che ha fatto i controlli

	senza := q10GiroCopia(t, "senza l'agente", pr, reg, thread, fuori, b)
	// T-Q10-08: i thread dell'oracolo esistono davvero nella copia _run, letti dalla sua fotografia.
	presenti := map[uuid.UUID]bool{}
	for _, th := range senza.foto.Thread {
		presenti[th.ID] = true
	}
	for _, id := range thread {
		if !presenti[id] {
			testutil.NonEseguita(t, "un thread dell'oracolo (voce export.thread) non è nella fotografia della copia _run (T-Q10-08)")
		}
	}
	if len(senza.foto.Thread) != len(thread) {
		t.Errorf("la fotografia della copia _run ha %d thread, l'oracolo %d", len(senza.foto.Thread), len(thread))
	}

	// Gli ingressi dell'agente, discordanti, su ogni messaggio dei thread. La pulizia è registrata dopo PoolCopiaDelDump:
	// gira prima del ricontrollo delle sentinelle (t.Cleanup è LIFO), che contano anche i candidati di codice.
	var analisi, candidati, triage []uuid.UUID
	t.Cleanup(func() {
		ctx := context.Background()
		if len(analisi) > 0 {
			if _, err := p.Exec(ctx, "DELETE FROM "+q10TabellaDellAgente+" WHERE analisi_id = ANY($1)", analisi); err != nil {
				t.Errorf("pulizia della copia _run, suggerimenti dell'agente: %v", err)
			}
		}
		if len(candidati) > 0 {
			if _, err := p.Exec(ctx, `DELETE FROM candidato_codice WHERE origine = 'agente' AND codice = $2 AND messaggio_id = ANY($1)`,
				candidati, q10CodiceDellAgente); err != nil {
				t.Errorf("pulizia della copia _run, candidati di codice: %v", err)
			}
		}
		if len(triage) > 0 {
			if _, err := p.Exec(ctx, `DELETE FROM proposta_triage WHERE fonte = 'agente' AND triage_id = ANY($1)`, triage); err != nil {
				t.Errorf("pulizia della copia _run, triage: %v", err)
			}
		}
		if n, err := q10ContaSullaCopia(ctx, p, "SELECT count(*) FROM "+q10TabellaDellAgente); err != nil || n != 0 {
			t.Errorf("la copia _run non è tornata pulita: suggerimenti dell'agente %d (%v)", n, err)
		}
		if n, err := q10ContaSullaCopia(ctx, p, q10SqlTriageDellAgente); err != nil || n != 0 {
			t.Errorf("la copia _run non è tornata pulita: triage di fonte agente %d (%v)", n, err)
		}
	})
	messaggi := 0
	for _, th := range senza.foto.Thread {
		for _, msg := range th.Messaggi {
			messaggi++
			analisi = append(analisi, testutil.AnalisiMessaggioDiProva(t, p, msg.ID, json.RawMessage(q10RisultatoDellAgente)))
			var id uuid.UUID
			if err := p.QueryRow(ctx, `INSERT INTO candidato_codice (messaggio_id, codice, ruolo, origine, punteggio)
				VALUES ($1, $2, 'prodotto', 'agente', 99) RETURNING messaggio_id`, msg.ID, q10CodiceDellAgente).Scan(&id); err != nil {
				t.Fatalf("candidato di codice dell'agente: %v", err)
			}
			candidati = append(candidati, id)
			if err := p.QueryRow(ctx, `INSERT INTO proposta_triage (messaggio_id, esito, identificativi, confidenza, fonte)
				VALUES ($1, 'aggancia', ARRAY[$2]::text[], 95, 'agente') RETURNING triage_id`, msg.ID, q10CodiceDellAgente).Scan(&id); err != nil {
				t.Fatalf("triage dell'agente: %v", err)
			}
			triage = append(triage, id)
		}
	}
	if messaggi == 0 {
		testutil.NonEseguita(t, "i thread dell'oracolo non hanno messaggi: gli ingressi dell'agente non hanno dove stare")
	}

	con := q10GiroCopia(t, "con l'agente", pr, reg, thread, fuori, b)

	// La fotografia esclude gli ingressi dell'agente (3.5, «Ingressi filtrati»): né i candidati né i triage scritti qui.
	for _, th := range con.foto.Thread {
		for _, c := range th.CandidatiCodice {
			if c.Codice == q10CodiceDellAgente || c.Origine == "agente" {
				t.Errorf("un candidato di codice dell'agente è nella fotografia")
			}
		}
		for _, x := range th.Triage {
			if slices.Contains(x.Identificativi, q10CodiceDellAgente) {
				t.Errorf("un triage dell'agente è nella fotografia")
			}
		}
	}
	if senza.impronta != con.impronta {
		t.Logf("l'impronta della fotografia cambia con gli ingressi dell'agente (non è un requisito: 3.5)")
	}
	if senza.esito.Impronta != con.esito.Impronta {
		t.Errorf("gli ingressi dell'agente cambiano l'impronta di valutazione")
	}
	for _, x := range []struct {
		nome string
		a, b map[uuid.UUID]string
	}{
		{"di confronto", senza.confronti, con.confronti},
		{"dei prodotti", senza.prodotti, con.prodotti},
		{"degli ancoraggi", senza.ancoraggi, con.ancoraggi},
		{"delle interpretazioni", senza.interpretazioni, con.interpretazioni},
	} {
		if n := q10Diverse(x.a, x.b); n > 0 {
			t.Errorf("gli ingressi dell'agente cambiano %d impronte %s", n, x.nome)
		}
	}
	if !reflect.DeepEqual(senza.perCodice, con.perCodice) {
		t.Errorf("gli ingressi dell'agente cambiano le diagnostiche per codice")
	}
	valutati := 0
	for _, et := range senza.esito.Thread {
		if et.Valutato {
			valutati++
		}
	}
	if valutati == 0 {
		t.Errorf("NON ESEGUITA: nessun thread dell'oracolo è valutato: E1 = E2 non dice niente del nuovo")
	}
	t.Logf("registro (gate «motore senza LLM»): thread dell'oracolo %d, valutati %d, messaggi con gli ingressi dell'agente %d, file interpretati %d",
		len(thread), valutati, messaggi, len(senza.interpretazioni))
}
