//go:build integrazione

// L4 — Fascicolo v3: i suffissi decorativi del cliente nelle proposte dei file. La coda che il CAD del
// cliente attacca al pezzo («77720000_PRT») non entra nel codice della proposta, ne' quando il file scende in
// staging o si carica a mano (scriviProposta), ne' quando arriva la lettura dell'analisi (propostaDaAnalisi).
// Le regole sono quelle della RFQ del file; se il file non e' ancora in una RFQ, quelle del cliente
// riconosciuto come controparte. Per chi non dichiara suffissi non cambia niente. La regola pura sta in
// classificazione (suffissi_test.go).

package workerapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/storage/staging"
	"promatec/cockpit/internal/platform/testutil"
)

const regoleConPRT = `{"suffissi_decorativi": ["_PRT"]}`

// preparaBancoSuffissi e' il banco dell'upload senza il suo disegno.pdf: il download che preparaBanco mette
// in coda lo prende un altro worker, cosi' il prossimo claim e' quello del file della prova.
func preparaBancoSuffissi(t *testing.T) *banco {
	t.Helper()
	b := preparaBanco(t, 0)
	b.claim("outlook@PC-ALTRO")
	return b
}

func (b *banco) clienteConRegole(cartella, regole string) uuid.UUID {
	b.t.Helper()
	var id uuid.UUID
	if err := b.pool.QueryRow(b.ctx, `INSERT INTO cliente (cartella_nas, ragione_sociale, regole) VALUES ($1, $1, $2) RETURNING cliente_id`,
		cartella, regole).Scan(&id); err != nil {
		b.t.Fatal(err)
	}
	return id
}

func (b *banco) rfqDelCliente(cliente uuid.UUID) uuid.UUID {
	b.t.Helper()
	var id uuid.UUID
	if err := b.pool.QueryRow(b.ctx, `INSERT INTO thread_offerta (cliente_id, canale, data_inizio) VALUES ($1, 'outlook', now()) RETURNING thread_id`,
		cliente).Scan(&id); err != nil {
		b.t.Fatal(err)
	}
	return id
}

// mettiNellaRfq aggancia il messaggio alla RFQ, come dopo la decisione dell'operatore.
func (b *banco) mettiNellaRfq(m db.Messaggio, thread uuid.UUID) db.Messaggio {
	b.t.Helper()
	if _, err := b.pool.Exec(b.ctx, `UPDATE messaggio SET thread_id = $2 WHERE messaggio_id = $1`, m.MessaggioID, thread); err != nil {
		b.t.Fatal(err)
	}
	return b.messaggioOra(m)
}

// daControparte scrive il cliente riconosciuto come controparte del messaggio.
func (b *banco) daControparte(m db.Messaggio, cliente uuid.UUID) db.Messaggio {
	b.t.Helper()
	if _, err := b.pool.Exec(b.ctx, `UPDATE messaggio SET controparte_tipo = 'cliente', controparte_cliente_id = $2,
		controparte_via = 'dominio', controparte_il = now() WHERE messaggio_id = $1`, m.MessaggioID, cliente); err != nil {
		b.t.Fatal(err)
	}
	return b.messaggioOra(m)
}

func (b *banco) messaggioOra(m db.Messaggio) db.Messaggio {
	b.t.Helper()
	x, err := b.q.GetMessaggio(b.ctx, m.MessaggioID)
	if err != nil {
		b.t.Fatal(err)
	}
	return x
}

// scendeInStaging porta l'allegato in staging per la strada vera: il download in coda, il claim del worker,
// l'upload del contenuto e il result. E' il result valido che fa partire dopoStaging, cioe' la proposta.
func (b *banco) scendeInStaging(a db.Allegato, m db.Messaggio, seme int64) {
	b.t.Helper()
	b.accodaDownload(a, m)
	j, tent := b.claimJob("outlook@PC-A")
	var p worker.PayloadStageAllegato
	if err := json.Unmarshal(j.Payload, &p); err != nil || p.AllegatoID != a.AllegatoID {
		b.t.Fatalf("il job preso non e' il download di %s: %+v (%v)", a.NomeFile, p, err)
	}
	contenuto, sha := contenutoCasuale(20_000, seme)
	stato(b.t, b.put(tent, a.AllegatoID, bytes.NewReader(contenuto), int64(len(contenuto))), 204)
	stato(b.t, b.result(tent, worker.RisultatoStage{AllegatoID: a.AllegatoID, Sha256: sha, Bytes: int64(len(contenuto))}), 204)
}

// letturaDellaProposta: "codice/rev", e i codici trovati nel nome se ci sono, della proposta dell'allegato.
func letturaDellaProposta(t *testing.T, b *banco, allegato uuid.UUID) string {
	t.Helper()
	var s string
	if err := b.pool.QueryRow(b.ctx, `SELECT coalesce(codice, '-') || '/' || coalesce(rev, '-') ||
		coalesce(' ' || (dettagli -> 'codici_nel_nome')::text, '') FROM documento_proposta WHERE allegato_id = $1`, allegato).Scan(&s); err != nil {
		t.Fatalf("proposta dell'allegato %s: %v", allegato, err)
	}
	return s
}

// Un file che scende in staging per un cliente che dichiara «_PRT» ha la proposta con il codice del pezzo:
// nella RFQ del cliente, e anche fuori da una RFQ, quando il cliente e' la controparte riconosciuta.
func TestUnFileCheScendeInStagingPerdeIlSuffissoDecorativo(t *testing.T) {
	b := preparaBancoSuffissi(t)
	acme := b.clienteConRegole("ACME", regoleConPRT)
	rfq := b.rfqDelCliente(acme)
	casi := []struct {
		nome   string
		inRfq  bool
		atteso string
	}{
		{"77720000_PRT.pdf", true, "77720000/-"},
		// la revisione prima del suffisso si rilegge: il pezzo e' 77730000, rev C
		{"77730000_C_PRT.stp", true, "77730000/C"},
		// il nome non e' un codice: i codici che contiene vanno nei dettagli, anche loro senza suffisso
		{"Offerta 77840000_PRT per staffe.pdf", true, `-/- ["77840000"]`},
		// fuori da una RFQ valgono le regole del cliente riconosciuto come controparte
		{"77722757_PRT.pdf", false, "77722757/-"},
	}
	for i, c := range casi {
		a, m := b.messaggioConAllegato(c.nome, estensione(c.nome))
		m = b.daControparte(m, acme)
		if c.inRfq {
			m = b.mettiNellaRfq(m, rfq)
		}
		b.scendeInStaging(a, m, int64(100+i))
		if got := letturaDellaProposta(t, b, a.AllegatoID); got != c.atteso {
			t.Errorf("%s: proposta %q, attesa %q", c.nome, got, c.atteso)
		}
		if a := b.allegatoOraDi(a.AllegatoID); !a.Sha256.Valid || !a.PathStaging.Valid {
			t.Errorf("%s: il file non e' sceso in staging (sha %v, path %v)", c.nome, a.Sha256, a.PathStaging)
		}
	}
	// la proposta di un file nella RFQ sta nella RFQ
	var nellaRfq int
	if err := b.pool.QueryRow(b.ctx, `SELECT count(*) FROM documento_proposta WHERE thread_id = $1`, rfq).Scan(&nellaRfq); err != nil {
		t.Fatal(err)
	}
	if nellaRfq != 3 {
		t.Errorf("proposte nella RFQ: %d, attese 3", nellaRfq)
	}
}

// Il controllo: lo stesso file, per un cliente che non dichiara suffissi o per un mittente che non si
// conosce, resta com'e'. Per un altro cliente «_PRT» puo' essere una parte vera del codice.
func TestSenzaSuffissiIlFileCheScendeTieneIlSuoCodice(t *testing.T) {
	b := preparaBancoSuffissi(t)
	beta := b.clienteConRegole("BETA", `{}`)
	rfq := b.rfqDelCliente(beta)
	casi := []struct {
		nome    string
		cliente bool
		atteso  string
	}{
		{"77720000_PRT.pdf", true, "77720000_PRT/-"},
		{"77730000_C_PRT.stp", true, "77730000_C_PRT/-"},
		{"Offerta 77840000_PRT per staffe.pdf", true, `-/- ["77840000_PRT"]`},
		// nessuna RFQ e nessun cliente riconosciuto: nessuna regola, il codice resta
		{"77722757_PRT.pdf", false, "77722757_PRT/-"},
	}
	for i, c := range casi {
		a, m := b.messaggioConAllegato(c.nome, estensione(c.nome))
		if c.cliente {
			m = b.mettiNellaRfq(b.daControparte(m, beta), rfq)
		}
		b.scendeInStaging(a, m, int64(200+i))
		if got := letturaDellaProposta(t, b, a.AllegatoID); got != c.atteso {
			t.Errorf("%s: proposta %q, attesa %q", c.nome, got, c.atteso)
		}
	}
}

// estensione e' quella del nome, come la scrive l'ingest.
func estensione(nome string) string {
	return strings.ToLower(strings.TrimPrefix(path.Ext(nome), "."))
}

func (b *banco) allegatoOraDi(id uuid.UUID) db.Allegato {
	b.t.Helper()
	a, err := b.q.GetAllegato(b.ctx, id)
	if err != nil {
		b.t.Fatal(err)
	}
	return a
}

// Un file caricato a mano nel Fascicolo (B8.7) fa la stessa strada: il suffisso si toglie, la revisione in
// coda si rilegge, e poi — come per ogni caricamento interno — non diventa la revisione del cliente ma
// resta nei dettagli.
func TestUnCaricamentoInternoPerdeIlSuffissoDecorativo(t *testing.T) {
	// "codice/rev/rev_letta" della proposta
	for _, c := range []struct {
		nome, regole, atteso string
	}{
		{"con la regola", regoleConPRT, "77730000/-/C"},
		{"senza la regola", `{}`, "77730000_C_PRT/-/-"},
	} {
		t.Run(c.nome, func(t *testing.T) {
			b := preparaBanco(t, 0)
			rfq := b.rfqDelCliente(b.clienteConRegole("ACME", c.regole))
			var u db.Utente
			if err := b.pool.QueryRow(b.ctx, `INSERT INTO utente (sigla, nome, ufficio) VALUES ('SX', 'Prova suffissi', 'Tecnico')
				RETURNING utente_id, nome`).Scan(&u.UtenteID, &u.Nome); err != nil {
				t.Fatal(err)
			}
			const nome = "77730000_C_PRT.stp"
			_, sha := contenutoCasuale(8_000, 300)
			percorso, err := staging.PercorsoContenuto(b.cartella, sha, nome)
			if err != nil {
				t.Fatal(err)
			}
			tx, err := b.pool.Begin(b.ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(b.ctx)
			q := db.New(tx)
			nota, err := fascicolo.NotaInterna(b.ctx, q, rfq, u)
			if err != nil {
				t.Fatal(err)
			}
			a, err := fascicolo.RegistraCaricamento(b.ctx, q, nota, u.UtenteID, fascicolo.Caricato{Nome: nome, Sha256: sha, Bytes: 8_000, Percorso: percorso})
			if err != nil {
				t.Fatal(err)
			}
			if err := b.s.DopoCaricamento(b.ctx, q, a.AllegatoID); err != nil {
				t.Fatalf("la strada di un caricamento interno: %v", err)
			}
			if err := tx.Commit(b.ctx); err != nil {
				t.Fatal(err)
			}
			var got string
			if err := b.pool.QueryRow(b.ctx, `SELECT coalesce(codice, '-') || '/' || coalesce(rev, '-') || '/' || coalesce(dettagli ->> 'rev_letta', '-')
				FROM documento_proposta WHERE allegato_id = $1 AND thread_id = $2`, a.AllegatoID, rfq).Scan(&got); err != nil {
				t.Fatalf("proposta del caricamento: %v", err)
			}
			if got != c.atteso {
				t.Errorf("proposta del caricamento %q, attesa %q", got, c.atteso)
			}
		})
	}
}

// ---------------------------------------------------------------- la lettura dell'analisi

// copiaDelDisegno e' una copia dello stesso contenuto in un messaggio suo: in una RFQ (o in nessuna), da un
// cliente riconosciuto (o da nessuno), con la sua proposta ancora aperta.
type copiaDelDisegno struct {
	nome     string
	allegato uuid.UUID
}

// La lettura del cartiglio arriva dal worker con il suffisso che il nome del file gli ha portato dentro
// («77720000_PRT»). Ogni proposta aperta dello stesso contenuto la riceve con le regole del SUO cliente (A15):
// quello della RFQ del file, o quello della controparte se il file non e' in una RFQ. I fatti restano come
// il worker li ha letti: l'interpretazione e' della proposta.
func TestLaLetturaDellAnalisiPerdeIlSuffissoDecorativoDelCliente(t *testing.T) {
	pool := testutil.Pool(t)
	testutil.SchemaPulito(t, pool)
	ctx := context.Background()
	q := db.New(pool)
	an := coda.Analizzatore{Versione: 1}
	s := &Server{Pool: pool, Log: testutil.LogSilenzioso(), Analizzatore: an}
	riga := func(sql string, arg ...any) uuid.UUID {
		t.Helper()
		var id uuid.UUID
		if err := pool.QueryRow(ctx, sql, arg...).Scan(&id); err != nil {
			t.Fatalf("%v\n%s", err, sql)
		}
		return id
	}
	facoltativo := func(id uuid.UUID) uuid.NullUUID { return uuid.NullUUID{UUID: id, Valid: id != uuid.Nil} }
	acme := riga(`INSERT INTO cliente (cartella_nas, ragione_sociale, regole) VALUES ('ACME', 'ACME', $1) RETURNING cliente_id`, regoleConPRT)
	beta := riga(`INSERT INTO cliente (cartella_nas, ragione_sociale, regole) VALUES ('BETA', 'Beta', '{}') RETURNING cliente_id`)
	rfqAcme := riga(`INSERT INTO thread_offerta (cliente_id, canale, data_inizio) VALUES ($1, 'outlook', now()) RETURNING thread_id`, acme)
	rfqBeta := riga(`INSERT INTO thread_offerta (cliente_id, canale, data_inizio) VALUES ($1, 'outlook', now()) RETURNING thread_id`, beta)

	n := 0
	copia := func(nome, sha string, thread, cliente uuid.UUID) copiaDelDisegno {
		n++
		conv := riga(`INSERT INTO conversazione (canale, chiave_esterna, primo_messaggio_il) VALUES ('outlook', $1, now()) RETURNING conversazione_id`,
			fmt.Sprintf("CONV-SUFF-%d", n))
		tipo := "sconosciuto"
		if cliente != uuid.Nil {
			tipo = "cliente"
		}
		msg := riga(`INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, direzione, data_evento, oggetto, thread_id,
			controparte_tipo, controparte_cliente_id) VALUES ('outlook', $1, $2, 'entrata', now(), 'RFQ suffissi', $3, $4, $5) RETURNING messaggio_id`,
			fmt.Sprintf("<suff-%d@acme.example>", n), conv, facoltativo(thread), tipo, facoltativo(cliente))
		al := riga(`INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, origine, bytes, sha256, path_staging, ricevuto_il)
			VALUES ($1, 1, 'disegno.pdf', 'pdf', 'file', 'outlook', 1000, $2, 'C:\staging\suffissi\disegno.pdf', now()) RETURNING allegato_id`, msg, sha)
		if _, err := pool.Exec(ctx, `INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, confidenza, fonte)
			VALUES ($1, $2, 'da_determinare', 40, 'estensione')`, al, facoltativo(thread)); err != nil {
			t.Fatal(err)
		}
		return copiaDelDisegno{nome: nome, allegato: al}
	}
	// applica e' il result del worker analisi per la prima copia: i fatti arrivano a tutte le proposte aperte
	applica := func(sha string, prima uuid.UUID, codice, rev string) {
		t.Helper()
		payload, _ := json.Marshal(worker.PayloadAnalizzaAllegato{AllegatoID: prima, Bytes: 1000, Sha256: sha, NomeFile: "disegno.pdf",
			VersioneAnalizzatore: 1, HashConfigurazione: an.Hash()})
		var jobID int64
		if err := pool.QueryRow(ctx, `INSERT INTO job (tipo, worker_tipo, payload, lease_s, durata_max_s, stato)
			VALUES ('analizza_allegato', 'analisi', $1, 120, 600, 'in_corso') RETURNING job_id`, payload).Scan(&jobID); err != nil {
			t.Fatal(err)
		}
		j, err := q.GetJob(ctx, jobID)
		if err != nil {
			t.Fatal(err)
		}
		dati, _ := json.Marshal(worker.RisultatoAnalisi{AllegatoID: prima, TipoProposto: "disegno_2d", Codice: codice, Rev: rev,
			Confidenza: 92, Fonte: "cartiglio", Dettagli: json.RawMessage(`{"cartiglio": true}`), VersioneAnalizzatore: 1, HashConfigurazione: an.Hash()})
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if err := s.applicaRisultato(ctx, db.New(tx), &j, dati, nil); err != nil {
			t.Fatalf("il risultato dell'analisi non si applica: %v", err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	lettura := func(al uuid.UUID) string {
		t.Helper()
		var v string
		if err := pool.QueryRow(ctx, `SELECT tipo_proposto::text || ':' || coalesce(codice, '-') || '/' || coalesce(rev, '-')
			FROM documento_proposta WHERE allegato_id = $1`, al).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}

	letture := []struct {
		nome, sha, codice, rev string
		conRegola, senza       string // la proposta attesa con e senza la regola del cliente
	}{
		{"cartiglio con la revisione a parte", "5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a", "77720000_PRT", "4",
			"disegno_2d:77720000/4", "disegno_2d:77720000_PRT/4"},
		{"revisione nella coda del codice", "5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b", "77730000_C_PRT", "",
			"disegno_2d:77730000/C", "disegno_2d:77730000_C_PRT/-"},
	}
	for _, l := range letture {
		copie := []struct {
			c      copiaDelDisegno
			regola bool
		}{
			{copia("nella RFQ di ACME", l.sha, rfqAcme, acme), true},
			{copia("fuori RFQ, controparte ACME", l.sha, uuid.Nil, acme), true},
			{copia("nella RFQ di Beta", l.sha, rfqBeta, beta), false},
			{copia("fuori RFQ, mittente sconosciuto", l.sha, uuid.Nil, uuid.Nil), false},
			// la RFQ decide, non il mittente: un file di ACME nella RFQ di Beta si legge con le regole di Beta
			{copia("nella RFQ di Beta, controparte ACME", l.sha, rfqBeta, acme), false},
		}
		applica(l.sha, copie[0].c.allegato, l.codice, l.rev)
		for _, x := range copie {
			atteso := l.senza
			if x.regola {
				atteso = l.conRegola
			}
			if got := lettura(x.c.allegato); got != atteso {
				t.Errorf("%s, %s: proposta %q, attesa %q", l.nome, x.c.nome, got, atteso)
			}
		}
		// il fatto non si tocca: il worker ha letto quel codice, e i fatti valgono per tutte le RFQ
		var letto string
		if err := pool.QueryRow(ctx, `SELECT fatti -> 'esito' ->> 'codice' FROM analisi_fatti WHERE sha256 = $1`, l.sha).Scan(&letto); err != nil {
			t.Fatal(err)
		}
		if letto != l.codice {
			t.Errorf("%s: i fatti dicono %q, devono restare la lettura del worker, %q", l.nome, letto, l.codice)
		}
	}
}

// La posta di un fornitore, non ancora in una RFQ: le regole sono quelle dei clienti che gli hanno chiesto
// qualcosa, come all'arrivo (ingest.Motori.PerFornitore). Il codice che l'ingest ha scritto senza suffisso non
// torna con il suffisso quando il file scende; un fornitore a cui ha chiesto solo chi non dichiara suffissi lo
// tiene.
func TestLaPostaDelFornitoreScendeConLeRegoleDeiSuoiClienti(t *testing.T) {
	b := preparaBancoSuffissi(t)
	fornitore := func(nome string, cliente uuid.UUID) uuid.UUID {
		t.Helper()
		var f uuid.UUID
		if err := b.pool.QueryRow(b.ctx, `INSERT INTO fornitore (ragione_sociale, tipo) VALUES ($1, 'processi') RETURNING fornitore_id`, nome).Scan(&f); err != nil {
			t.Fatal(err)
		}
		if _, err := b.pool.Exec(b.ctx, `INSERT INTO richiesta_fornitore (thread_id, fornitore_id, stato, inviata_il) VALUES ($1, $2, 'inviata', now())`,
			b.rfqDelCliente(cliente), f); err != nil {
			t.Fatal(err)
		}
		return f
	}
	daFornitore := func(m db.Messaggio, f uuid.UUID) db.Messaggio {
		t.Helper()
		if _, err := b.pool.Exec(b.ctx, `UPDATE messaggio SET controparte_tipo = 'fornitore', controparte_fornitore_id = $2,
			controparte_via = 'dominio', controparte_il = now() WHERE messaggio_id = $1`, m.MessaggioID, f); err != nil {
			t.Fatal(err)
		}
		return b.messaggioOra(m)
	}
	minuterie := fornitore("Minuterie Esempio", b.clienteConRegole("ACME", regoleConPRT))
	altro := fornitore("Altro", b.clienteConRegole("BETA", `{}`))
	for i, c := range []struct {
		fornitore    uuid.UUID
		nome, atteso string
	}{{minuterie, "77720000_PRT.pdf", "77720000/-"}, {altro, "77730000_PRT.pdf", "77730000_PRT/-"}} {
		a, m := b.messaggioConAllegato(c.nome, "pdf")
		m = daFornitore(m, c.fornitore)
		b.scendeInStaging(a, m, int64(300+i))
		if got := letturaDellaProposta(t, b, a.AllegatoID); got != c.atteso {
			t.Errorf("fornitore %d: proposta %q, attesa %q", i, got, c.atteso)
		}
	}
}
