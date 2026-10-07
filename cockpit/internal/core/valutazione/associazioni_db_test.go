//go:build integrazione

// L4 — lo smistamento sulla fotografia letta dal database di prova (B6, V2; PO-14, PO-27 e PO-39 in L4 sintetica; R105
// [U], E1R §6; T-E1R-10, T-B0-32; R93, T-E1-11): la scena ACME di B1 (costruisciScenaDB) con un secondo messaggio che non
// nomina prodotti, un file orfano e un file con la proposta scartata da una persona, scritta come la scrive il gesto
// «scarta» del legacy (stato scartata, deciso_da, deciso_il: solo nel DB). Letta con caricatore.Carica in sola lettura,
// poi Calcola, che non parla con il database. Lo scartato è escluso e terminale, fuori da «da smistare», e la voce del 2D
// del finito che mancava resta manca; l'orfano è in «da smistare» con l'avviso. Poi lo stesso orfano scartato con il
// gesto esce da «da smistare» e dagli avvisi.
//
// I clienti, i codici, i nomi dei file e gli ID sono inventati (ACME, 712xxxx, acme.example): il repository è
// pubblico.

package valutazione_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/fotorfq/caricatore"
	"promatec/cockpit/internal/core/valutazione"
	"promatec/cockpit/internal/platform/testutil"
)

// TestL4LoScartoELOrfanoDalDatabase (PO-14, PO-27, PO-39 in L4 sintetica): lo scarto legacy scritto sul database di prova
// è l'unico modo in cui un file è escluso; l'orfano fa l'avviso finché una persona non lo scarta.
func TestL4LoScartoELOrfanoDalDatabase(t *testing.T) {
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	s := costruisciScenaDB(t, p)
	ctx := context.Background()
	esegui := func(sql string, arg ...any) {
		t.Helper()
		if _, err := p.Exec(ctx, sql, arg...); err != nil {
			t.Fatalf("scena: %v\n%s", err, sql)
		}
	}
	riga := func(dst any, sql string, arg ...any) {
		t.Helper()
		if err := p.QueryRow(ctx, sql, arg...).Scan(dst); err != nil {
			t.Fatalf("scena: %v\n%s", err, sql)
		}
	}
	var conv, m2, aOrfano, aScartato uuid.UUID
	riga(&conv, `INSERT INTO conversazione (canale, chiave_esterna, primo_messaggio_il) VALUES ('outlook', 'C-ACME-B6V2', now()) RETURNING conversazione_id`)
	riga(&m2, `INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, thread_id, aggancio, agganciato_da, agganciato_il, direzione,
		data_evento, oggetto, corpo_testo, controparte_tipo, controparte_cliente_id) VALUES ('outlook', '<b6v2@acme.example>', $1, $2, 'operatore', $3,
		now() - interval '50 minutes', 'entrata', now() - interval '60 minutes', 'File ACME26-030',
		E'Buongiorno,\r\nin allegato i file.\r\nGrazie', 'cliente', $4) RETURNING messaggio_id`, conv, s.thread, s.operatore, s.cliente)
	allegato := func(dst *uuid.UUID, indice int, nome, sha string) {
		t.Helper()
		riga(dst, `INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, origine, bytes, sha256, stato, ricevuto_il)
			VALUES ($1, $2, $3, 'pdf', 'file', 'outlook', 100, $4, 'analizzato', now()) RETURNING allegato_id`, m2, indice, nome, sha)
	}
	allegato(&aOrfano, 1, "allegato-acme.pdf", sha("5"))
	allegato(&aScartato, 2, "disegno-scartato.pdf", sha("6"))
	esegui(`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, confidenza, fonte) VALUES ($1, $2, 'altro', 30, 'estensione')`,
		aOrfano, s.thread)
	// Lo scarto come lo scrive il gesto «scarta» del legacy (DecidiProposta): la proposta scartata, con chi e quando.
	esegui(`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, componente_id, confidenza, fonte, stato, deciso_da, deciso_il)
		VALUES ($1, $2, 'disegno_2d', '7120300', $3, 50, 'estensione', 'scartata', $4, now())`, aScartato, s.thread, s.finito, s.operatore)

	pr, reg := testutil.PoolConRegistro(t, testutil.DSN(t))
	r := insiemeDelDB(t, s.cliente)
	esito := func(nome string) valutazione.EsitoThread {
		t.Helper()
		f, err := caricatore.Carica(ctx, pr, caricatore.Richiesta{Thread: []uuid.UUID{s.thread}})
		if err != nil {
			t.Fatalf("%s: Carica: %v", nome, err)
		}
		reg.Azzera()
		e := calcola(t, f, r, valutazione.Ingressi{})
		if len(reg.Voci()) != 0 {
			t.Errorf("%s: Calcola ha parlato con il database: %v", nome, reg.Voci())
		}
		if len(e.Thread) != 1 || !e.Thread[0].Valutato {
			t.Fatalf("%s: esito %+v", nome, e.Thread)
		}
		return e.Thread[0]
	}

	et := esito("lo scarto e l'orfano")
	sc := associazioneDi(t, et, aScartato)
	if !sc.Esclusa || !sc.Terminale || sc.Manuale != nil || sc.Perimetro != valutazione.PerimetroDentro {
		t.Errorf("lo scartato %+v", sc)
	}
	if _, ok := daSmistareDi(et, aScartato); ok {
		t.Error("lo scartato è da smistare")
	}
	rifFinito := "componente:" + s.finito.String()
	finito := prodottoDi(t, et, rifFinito)
	if nonTerminale(finito.Smistamento, aScartato) || esitoDi(voceDi(t, finito.Documenti, s.finito, "disegno_2d")) != "manca/nessun_documento" {
		t.Errorf("lo scartato tiene aperto lo smistamento (%+v) o soddisfa la voce del 2D (%s)", finito.Smistamento,
			esitoDi(voceDi(t, finito.Documenti, s.finito, "disegno_2d")))
	}
	o, ok := daSmistareDi(et, aOrfano)
	if !ok || !o.Orfano || len(o.Prodotti) != 0 || o.Motivo != valutazione.MotivoSmistamentoNessunCandidato ||
		!contieneRif(et.Fascicolo.Avvisi, "orfano:"+aOrfano.String()) {
		t.Errorf("l'orfano %+v %v, avvisi %v", o, ok, et.Fascicolo.Avvisi)
	}
	for _, pv := range et.ProdottiValutati {
		if nonTerminale(pv.Smistamento, aOrfano) {
			t.Errorf("l'orfano blocca %s", pv.Rif)
		}
	}

	// Lo stesso orfano scartato con il gesto, sul database.
	esegui(`UPDATE documento_proposta SET stato = 'scartata', deciso_da = $2, deciso_il = now() WHERE allegato_id = $1`, aOrfano, s.operatore)
	et = esito("l'orfano scartato")
	if _, ok := daSmistareDi(et, aOrfano); ok || contieneRif(et.Fascicolo.Avvisi, "orfano:"+aOrfano.String()) || !associazioneDi(t, et, aOrfano).Esclusa {
		t.Errorf("l'orfano scartato: avvisi %v", et.Fascicolo.Avvisi)
	}
	var stato string
	riga(&stato, `SELECT stato::text FROM documento_proposta WHERE allegato_id = $1`, aScartato)
	if stato != "scartata" {
		t.Errorf("la lettura ha cambiato il database: %s", stato)
	}
}
