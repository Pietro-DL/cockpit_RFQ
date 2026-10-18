//go:build integrazione

// L4 — la bozza della distinta sul database di prova (giro 5, A1d, Q1; direttiva 2026-10-12-06, B1: «query SQL
// dirette di creazione/lettura/salvataggio/abbandono con rilettura DB», «persistenza di bozza con 35 nodi iniziali,
// tre esclusioni, due aggiunte, due ricodifiche e spostamenti: … riavvio server e ripristino fedele»; correzioni
// della revisione del codice RC-01…RC-19). Solo dati sintetici (ACME, example.com). Le rotte HTTP R1-R3 sono di B2:
// qui si prova il package che useranno.

package distintabozza

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/testutil"
)

// La scena del database: una RFQ ACME con i 35 componenti della scena (il finito e 34 registrati, con gli archi
// nella working), un messaggio con due allegati STEP, un secondo finito e un sottoassieme senza bozza, un finito
// archiviato; una seconda RFQ con un finito e un allegato.
type scenaDB struct {
	utente, thread, thread2 uuid.UUID
	prodotto                uuid.UUID
	altroFinito             uuid.UUID
	sotto                   uuid.UUID
	archiviato              uuid.UUID
	prodottoT2              uuid.UUID
	allegato, allegato2     uuid.UUID
	allegatoT2              uuid.UUID
	sc                      scena
}

func preparaScena(t *testing.T, p *pgxpool.Pool) scenaDB {
	t.Helper()
	ctx := context.Background()
	s := scenaDB{utente: idDi("utente"), thread: idDi("rfq"), thread2: idDi("rfq2"), prodotto: idDi("prodotto"),
		altroFinito: idDi("altro-finito"), sotto: idDi("sotto-libero"), archiviato: idDi("archiviato"), prodottoT2: idDi("prodotto-t2"),
		allegato: idDi("allegato"), allegato2: idDi("allegato2"), allegatoT2: idDi("allegato-t2")}
	s.sc = scena35(s.prodotto, idDi)
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	ex := func(sql string, args ...any) {
		t.Helper()
		if _, err := tx.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%v\n%s", err, sql)
		}
	}
	ex(`INSERT INTO utente (utente_id, sigla, nome, ufficio, ruolo) VALUES ($1, 'DBZ', 'Prova bozza', 'UT', 'admin')`, s.utente)
	ex(`INSERT INTO cliente (cliente_id, cartella_nas, ragione_sociale) VALUES ($1, 'ACMEBZ', 'ACME S.p.A.')`, idDi("cliente"))
	ex(`INSERT INTO thread_offerta (thread_id, cliente_id, canale, data_inizio, oggetto) VALUES ($1, $3, 'outlook', now(), 'RFQ ACME-1000'), ($2, $3, 'outlook', now(), 'RFQ ACME-7000')`,
		s.thread, s.thread2, idDi("cliente"))
	for i, th := range []uuid.UUID{s.thread, s.thread2} {
		cv, m := idDi(fmt.Sprintf("conv-%d", i)), idDi(fmt.Sprintf("msg-%d", i))
		ex(`INSERT INTO conversazione (conversazione_id, canale, chiave_esterna, thread_id, primo_messaggio_il) VALUES ($1, 'outlook', $2, $3, now())`,
			cv, fmt.Sprintf("conv-bozza-%d@example.com", i), th)
		ex(`INSERT INTO messaggio (messaggio_id, canale, chiave_esterna, conversazione_id, thread_id, direzione, data_evento, oggetto)
			VALUES ($1, 'outlook', $2, $3, $4, 'entrata', now(), 'RFQ ACME')`, m, fmt.Sprintf("msg-bozza-%d@example.com", i), cv, th)
		if i == 0 {
			ex(`INSERT INTO allegato (allegato_id, messaggio_id, indice, nome_file, estensione, sha256, ricevuto_il) VALUES
				($1, $3, 1, 'ACM-1000.stp', 'stp', repeat('b', 64), now()), ($2, $3, 2, 'ACM-1000_bis.stp', 'stp', repeat('c', 64), now())`,
				s.allegato, s.allegato2, m)
		} else {
			ex(`INSERT INTO allegato (allegato_id, messaggio_id, indice, nome_file, estensione, sha256, ricevuto_il) VALUES ($1, $2, 1, 'ACM-7000.stp', 'stp', repeat('e', 64), now())`,
				s.allegatoT2, m)
		}
	}
	for _, n := range s.sc.snap.Nodi {
		var rev, descr pgtype.Text
		if n.Rev != "" {
			rev = pgtype.Text{String: n.Rev, Valid: true}
		}
		if n.Descrizione != "" {
			descr = pgtype.Text{String: n.Descrizione, Valid: true}
		}
		ex(`INSERT INTO componente (componente_id, thread_id, codice, rev, descrizione, tipo, origine, confermato_da) VALUES ($1, $2, $3, $4, $5, $6, 'manuale', $7)`,
			*n.ComponenteID, s.thread, n.Codice, rev, descr, n.Tipo, s.utente)
	}
	ex(`INSERT INTO componente (componente_id, thread_id, codice, tipo, origine, confermato_da) VALUES
		($1, $5, 'ACM-6000', 'finito', 'manuale', $7), ($2, $5, 'ACM-6100', 'sottoassieme', 'manuale', $7),
		($3, $5, 'ACM-6900', 'finito', 'manuale', $7), ($4, $6, 'ACM-7000', 'finito', 'manuale', $7)`,
		s.altroFinito, s.sotto, s.archiviato, s.prodottoT2, s.thread, s.thread2, s.utente)
	ex(`UPDATE componente SET archiviato_il = now(), archiviato_da = $2, motivo_archiviazione = 'prova' WHERE componente_id = $1`, s.archiviato, s.utente)
	// la working del prodotto: gli archi della scena, registrati
	for _, a := range s.sc.snap.Archi {
		ex(`INSERT INTO componente_relazione (thread_id, padre_id, figlio_id, qta, origine, confermato_da) VALUES ($1, $2, $3, $4, 'manuale', $5)`,
			s.thread, s.sc.componenti[a.Padre], s.sc.componenti[a.Figlio], a.Qta, s.utente)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return s
}

func poolPulito(t *testing.T) (*pgxpool.Pool, scenaDB) {
	t.Helper()
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	return p, preparaScena(t, p)
}

// salva: un salvataggio della scena, con il vocabolario 1.
func (s scenaDB) salva(ctx context.Context, p *pgxpool.Pool, prodotto uuid.UUID, bozza *uuid.UUID, versione int, snap Snapshot, azioni []Azione) (Salvato, error) {
	return Salva(ctx, p, Salvataggio{ThreadID: s.thread, ProdottoID: prodotto, UtenteID: s.utente, BozzaID: bozza, VersioneAttesa: versione,
		Vocabolario: Vocabolario, Snapshot: mustJSON(snap), Azioni: azioni})
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

// soloRadice: la partenza «vuota» di un finito della scena DB senza rev ne' descrizione (ACM-6000, ACM-6500).
func soloRadice(prodotto uuid.UUID, codice string) Snapshot {
	return Snapshot{Formato: 2, ProdottoID: prodotto, VersioneBomBase: &VersioneBomBase{}, FirmaBase: firmaProva,
		Nodi: []Nodo{{ChiaveBozza: "r", ComponenteID: pu(prodotto), Op: OpRiusa, Codice: codice, Tipo: "finito", Origine: "registrato"}}}
}

// ------------------------------------------------------------------ le query, una per una, con la rilettura

func TestLeQueryDellaBozzaConRiletturaDalDatabase(t *testing.T) {
	p, s := poolPulito(t)
	ctx := context.Background()
	q := db.New(p)

	// creazione: testata + versione 1 + evento, in una transazione
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	qt := db.New(tx)
	if _, err := qt.BloccaThread(ctx, s.thread); err != nil {
		t.Fatal(err)
	}
	r, err := qt.InsertDistintaBozza(ctx, db.InsertDistintaBozzaParams{ThreadID: s.thread, ProdottoID: s.altroFinito, Formato: FormatoSnapshot, BaseNumero: 0,
		BaseContatore: 3, FirmaBase: firmaProva, FonteAllegatoID: uuid.NullUUID{UUID: s.allegato, Valid: true},
		FonteSha256: pgtype.Text{String: shaProva('b'), Valid: true}, FonteGruppo: pgtype.Text{String: "cand:x:ACM-6000", Valid: true}, UtenteID: s.utente})
	if err != nil {
		t.Fatalf("InsertDistintaBozza: %v", err)
	}
	snap1 := json.RawMessage(`{"formato":2,"prodotto_id":"` + s.altroFinito.String() + `","nodi":[]}`)
	if _, err := qt.InsertDistintaBozzaVersione(ctx, db.InsertDistintaBozzaVersioneParams{BozzaID: r.BozzaID, Versione: 1, Snapshot: snap1,
		Impronta: shaProva('1'), BaseNumero: 0, BaseContatore: 3, FirmaBase: firmaProva, FonteAllegatoID: uuid.NullUUID{UUID: s.allegato, Valid: true},
		FonteSha256: pgtype.Text{String: shaProva('b'), Valid: true}, FonteGruppo: pgtype.Text{String: "cand:x:ACM-6000", Valid: true}, SalvataDa: s.utente}); err != nil {
		t.Fatalf("InsertDistintaBozzaVersione: %v", err)
	}
	dopo := json.RawMessage(`{"partenza":"vuota"}`)
	if err := qt.InsertDistintaBozzaEvento(ctx, db.InsertDistintaBozzaEventoParams{BozzaID: r.BozzaID, N: 1, Versione: 1, Vocabolario: 1,
		Azione: "bozza_aperta", Dopo: &dopo, Origine: "utente", UtenteID: s.utente}); err != nil {
		t.Fatalf("InsertDistintaBozzaEvento: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("COMMIT della creazione: %v", err)
	}

	// rilettura
	b, err := q.GetDistintaBozzaAperta(ctx, db.GetDistintaBozzaApertaParams{ThreadID: s.thread, ProdottoID: s.altroFinito})
	if err != nil || b.BozzaID != r.BozzaID || b.Versione != 1 || b.Stato != StatoAperta || b.BaseContatore != 3 || b.FonteGruppo.String != "cand:x:ACM-6000" {
		t.Fatalf("GetDistintaBozzaAperta: %+v %v", b, err)
	}
	if !b.CreataIl.Equal(r.CreataIl) || b.CreataDa != s.utente {
		t.Errorf("creazione riletta diversa: %v / %v", b.CreataIl, r.CreataIl)
	}
	if _, err := q.GetDistintaBozza(ctx, db.GetDistintaBozzaParams{BozzaID: r.BozzaID, ThreadID: s.thread2}); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("una bozza si legge da un'altra RFQ: %v", err)
	}
	v, err := q.GetDistintaBozzaVersione(ctx, db.GetDistintaBozzaVersioneParams{BozzaID: r.BozzaID, ThreadID: s.thread, Versione: 1})
	if err != nil || v.Impronta != shaProva('1') || !strings.Contains(string(v.Snapshot), `"formato": 2`) || v.SalvataDa != s.utente {
		t.Fatalf("GetDistintaBozzaVersione: %+v %v", v, err)
	}
	ev, err := q.ListDistintaBozzaEventi(ctx, db.ListDistintaBozzaEventiParams{BozzaID: r.BozzaID, ThreadID: s.thread, VersioneMax: 1, DopoN: 0, Limite: 10})
	if err != nil || len(ev) != 1 || ev[0].Azione != "bozza_aperta" || ev[0].Dopo == nil || ev[0].Quando.IsZero() {
		t.Fatalf("ListDistintaBozzaEventi: %+v %v", ev, err)
	}
	if e1, err := q.GetDistintaBozzaEvento(ctx, db.GetDistintaBozzaEventoParams{BozzaID: r.BozzaID, ThreadID: s.thread, N: 1}); err != nil || e1.Azione != "bozza_aperta" {
		t.Errorf("GetDistintaBozzaEvento: %+v %v", e1, err)
	}
	if n, err := q.UltimoEventoDistintaBozza(ctx, db.UltimoEventoDistintaBozzaParams{BozzaID: r.BozzaID, ThreadID: s.thread}); err != nil || n != 1 {
		t.Errorf("UltimoEventoDistintaBozza: %d %v", n, err)
	}
	if n, err := q.UltimoEventoDistintaBozza(ctx, db.UltimoEventoDistintaBozzaParams{BozzaID: uuid.New(), ThreadID: s.thread}); err != nil || n != 0 {
		t.Errorf("UltimoEventoDistintaBozza di una bozza senza eventi: %d %v", n, err)
	}
	if n, err := q.ContaDistintaBozzaEventi(ctx, db.ContaDistintaBozzaEventiParams{BozzaID: r.BozzaID, ThreadID: s.thread, VersioneMax: 1}); err != nil || n != 1 {
		t.Errorf("ContaDistintaBozzaEventi: %d %v", n, err)
	}
	// RC-19: versioni ed eventi portano la RFQ: da un'altra RFQ non si trovano
	if _, err := q.GetDistintaBozzaVersione(ctx, db.GetDistintaBozzaVersioneParams{BozzaID: r.BozzaID, ThreadID: s.thread2, Versione: 1}); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("GetDistintaBozzaVersione da un'altra RFQ: %v", err)
	}
	if _, err := q.GetDistintaBozzaEvento(ctx, db.GetDistintaBozzaEventoParams{BozzaID: r.BozzaID, ThreadID: s.thread2, N: 1}); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("GetDistintaBozzaEvento da un'altra RFQ: %v", err)
	}
	if ee, err := q.ListDistintaBozzaEventi(ctx, db.ListDistintaBozzaEventiParams{BozzaID: r.BozzaID, ThreadID: s.thread2, VersioneMax: 9, Limite: 10}); err != nil || len(ee) != 0 {
		t.Errorf("ListDistintaBozzaEventi da un'altra RFQ: %d %v", len(ee), err)
	}
	if vv, err := q.ListDistintaBozzaVersioni(ctx, db.ListDistintaBozzaVersioniParams{BozzaID: r.BozzaID, ThreadID: s.thread2}); err != nil || len(vv) != 0 {
		t.Errorf("ListDistintaBozzaVersioni da un'altra RFQ: %d %v", len(vv), err)
	}
	if n, err := q.UltimoEventoDistintaBozza(ctx, db.UltimoEventoDistintaBozzaParams{BozzaID: r.BozzaID, ThreadID: s.thread2}); err != nil || n != 0 {
		t.Errorf("UltimoEventoDistintaBozza da un'altra RFQ: %d %v", n, err)
	}
	if n, err := q.ContaDistintaBozzaEventi(ctx, db.ContaDistintaBozzaEventiParams{BozzaID: r.BozzaID, ThreadID: s.thread2, VersioneMax: 9}); err != nil || n != 0 {
		t.Errorf("ContaDistintaBozzaEventi da un'altra RFQ: %d %v", n, err)
	}

	// salvataggio: versione 2 (la testata la segue) + evento 2, sotto i due lucchetti
	tx, err = p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	qt = db.New(tx)
	if _, err := qt.BloccaThread(ctx, s.thread); err != nil {
		t.Fatal(err)
	}
	if bb, err := qt.BloccaDistintaBozza(ctx, db.BloccaDistintaBozzaParams{BozzaID: r.BozzaID, ThreadID: s.thread}); err != nil || bb.Versione != 1 {
		t.Fatalf("BloccaDistintaBozza: %+v %v", bb, err)
	}
	if _, err := qt.InsertDistintaBozzaVersione(ctx, db.InsertDistintaBozzaVersioneParams{BozzaID: r.BozzaID, Versione: 2, Snapshot: snap1,
		Impronta: shaProva('2'), BaseNumero: 0, BaseContatore: 4, FirmaBase: firmaProva, FonteGruppo: pgtype.Text{String: "bom", Valid: true}, SalvataDa: s.utente}); err != nil {
		t.Fatal(err)
	}
	if err := qt.InsertDistintaBozzaEvento(ctx, db.InsertDistintaBozzaEventoParams{BozzaID: r.BozzaID, N: 2, Versione: 2, Vocabolario: 1,
		Azione: "sorgente_cambiata", Origine: "utente", Motivo: pgtype.Text{String: "la fonte giusta e' la BOM registrata", Valid: true}, UtenteID: s.utente}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("COMMIT del salvataggio: %v", err)
	}
	b, _ = q.GetDistintaBozza(ctx, db.GetDistintaBozzaParams{BozzaID: r.BozzaID, ThreadID: s.thread})
	if b.Versione != 2 || b.BaseContatore != 4 || b.FonteAllegatoID.Valid || b.FonteGruppo.String != "bom" {
		t.Errorf("la testata non segue la versione 2: %+v", b)
	}
	vv, err := q.ListDistintaBozzaVersioni(ctx, db.ListDistintaBozzaVersioniParams{BozzaID: r.BozzaID, ThreadID: s.thread})
	if err != nil || len(vv) != 2 || vv[0].Versione != 1 || vv[1].Versione != 2 || vv[1].Impronta != shaProva('2') {
		t.Errorf("ListDistintaBozzaVersioni: %d %v", len(vv), err)
	}
	// gli eventi fino a una versione, e a pagine
	if ee, _ := q.ListDistintaBozzaEventi(ctx, db.ListDistintaBozzaEventiParams{BozzaID: r.BozzaID, ThreadID: s.thread, VersioneMax: 1, Limite: 10}); len(ee) != 1 {
		t.Errorf("eventi fino alla versione 1: %d", len(ee))
	}
	if ee, _ := q.ListDistintaBozzaEventi(ctx, db.ListDistintaBozzaEventiParams{BozzaID: r.BozzaID, ThreadID: s.thread, VersioneMax: 2, DopoN: 1, Limite: 10}); len(ee) != 1 || ee[0].N != 2 {
		t.Errorf("eventi dopo il primo: %+v", ee)
	}
	if ee, _ := q.ListDistintaBozzaEventi(ctx, db.ListDistintaBozzaEventiParams{BozzaID: r.BozzaID, ThreadID: s.thread, VersioneMax: 2, Limite: 1}); len(ee) != 1 || ee[0].N != 1 {
		t.Errorf("pagina di un evento: %+v", ee)
	}

	// abbandono: con la versione vecchia 0 righe, con quella giusta 1; poi chiusa
	if n, err := q.AbbandonaDistintaBozza(ctx, db.AbbandonaDistintaBozzaParams{ChiusaDa: uuid.NullUUID{UUID: s.utente, Valid: true},
		Motivo: pgtype.Text{String: "rifatta da capo", Valid: true}, BozzaID: r.BozzaID, ThreadID: s.thread, Versione: 1}); err != nil || n != 0 {
		t.Errorf("abbandono con la versione vecchia: %d righe, %v", n, err)
	}
	if n, err := q.AbbandonaDistintaBozza(ctx, db.AbbandonaDistintaBozzaParams{ChiusaDa: uuid.NullUUID{UUID: s.utente, Valid: true},
		Motivo: pgtype.Text{String: "rifatta da capo", Valid: true}, BozzaID: r.BozzaID, ThreadID: s.thread2, Versione: 2}); err != nil || n != 0 {
		t.Errorf("abbandono da un'altra RFQ: %d righe, %v", n, err)
	}
	if n, err := q.AbbandonaDistintaBozza(ctx, db.AbbandonaDistintaBozzaParams{ChiusaDa: uuid.NullUUID{UUID: s.utente, Valid: true},
		Motivo: pgtype.Text{String: "rifatta da capo", Valid: true}, BozzaID: r.BozzaID, ThreadID: s.thread, Versione: 2}); err != nil || n != 1 {
		t.Fatalf("abbandono: %d righe, %v", n, err)
	}
	b, _ = q.GetDistintaBozza(ctx, db.GetDistintaBozzaParams{BozzaID: r.BozzaID, ThreadID: s.thread})
	if b.Stato != StatoAbbandonata || b.ChiusaIl == nil || !b.ChiusaDa.Valid || b.MotivoChiusura.String != "rifatta da capo" {
		t.Errorf("bozza abbandonata riletta: %+v", b)
	}
	if _, err := q.GetDistintaBozzaAperta(ctx, db.GetDistintaBozzaApertaParams{ThreadID: s.thread, ProdottoID: s.altroFinito}); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("dopo l'abbandono c'e' ancora una bozza aperta: %v", err)
	}
	tutte, err := q.ListDistinteBozzeProdotto(ctx, db.ListDistinteBozzeProdottoParams{ThreadID: s.thread, ProdottoID: s.altroFinito})
	if err != nil || len(tutte) != 1 {
		t.Errorf("ListDistinteBozzeProdotto: %d %v", len(tutte), err)
	}

	// le letture d'appoggio
	pr, err := q.GetProdottoDellaRfq(ctx, db.GetProdottoDellaRfqParams{ThreadID: s.thread, ComponenteID: s.archiviato})
	if err != nil || pr.Tipo != db.TipoComponenteFinito || pr.ArchiviatoIl == nil {
		t.Errorf("GetProdottoDellaRfq: %+v %v", pr, err)
	}
	if pr, err := q.GetProdottoDellaRfq(ctx, db.GetProdottoDellaRfqParams{ThreadID: s.thread, ComponenteID: s.prodotto}); err != nil || pr.Rev.String != "A" || pr.Descrizione.String != "Gruppo base di prova" {
		t.Errorf("GetProdottoDellaRfq con rev e descrizione: %+v %v", pr, err)
	}
	if _, err := q.GetProdottoDellaRfq(ctx, db.GetProdottoDellaRfqParams{ThreadID: s.thread, ComponenteID: s.prodottoT2}); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("GetProdottoDellaRfq trova un prodotto di un'altra RFQ: %v", err)
	}
	a, err := q.GetAllegatoDellaRfq(ctx, db.GetAllegatoDellaRfqParams{ThreadID: s.thread, AllegatoID: s.allegato})
	if err != nil || strings.TrimSpace(a.Sha256.String) != shaProva('b') {
		t.Errorf("GetAllegatoDellaRfq: %+v %v", a, err)
	}
	if _, err := q.GetAllegatoDellaRfq(ctx, db.GetAllegatoDellaRfqParams{ThreadID: s.thread, AllegatoID: s.allegatoT2}); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("GetAllegatoDellaRfq trova un allegato di un'altra RFQ: %v", err)
	}
	// il perimetro registrato del prodotto: i 35 componenti e i 34 archi della working
	cc, err := q.ListPerimetroRegistrato(ctx, db.ListPerimetroRegistratoParams{ThreadID: s.thread, ProdottoID: s.prodotto})
	if err != nil || len(cc) != 35 {
		t.Errorf("ListPerimetroRegistrato: %d %v", len(cc), err)
	}
	aa, err := q.ListArchiPerimetroRegistrato(ctx, db.ListArchiPerimetroRegistratoParams{ThreadID: s.thread, ProdottoID: s.prodotto})
	if err != nil || len(aa) != 34 {
		t.Errorf("ListArchiPerimetroRegistrato: %d %v", len(aa), err)
	}
	if cc, _ := q.ListPerimetroRegistrato(ctx, db.ListPerimetroRegistratoParams{ThreadID: s.thread2, ProdottoID: s.prodotto}); len(cc) != 0 {
		t.Errorf("perimetro letto da un'altra RFQ: %d", len(cc))
	}
	if cc, _ := q.ListPerimetroRegistrato(ctx, db.ListPerimetroRegistratoParams{ThreadID: s.thread, ProdottoID: s.altroFinito}); len(cc) != 1 {
		t.Errorf("perimetro di un finito senza archi: %d", len(cc))
	}
	if _, err := q.GetConfermaStrutturaleBase(ctx, db.GetConfermaStrutturaleBaseParams{ThreadID: s.thread, ConfermaID: uuid.New()}); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("GetConfermaStrutturaleBase di una conferma che non c'e': %v", err)
	}
	wc, err := q.GetWorkingContatore(ctx, s.thread2)
	if err != nil || wc.Struttura != 1 || wc.Dati != 0 {
		t.Errorf("GetWorkingContatore della RFQ 2 (una transazione di nascita): %+v %v", wc, err)
	}
	if wc, err := q.GetWorkingContatore(ctx, uuid.New()); err != nil || wc.Struttura != 0 || wc.Dati != 0 {
		t.Errorf("GetWorkingContatore senza riga: %+v %v", wc, err)
	}
}

// ------------------------------------------------------------------ i 35 nodi, il riavvio, il ripristino fedele

func TestLaBozzaDei35NodiSiRipristinaFedeleDopoIlRiavvio(t *testing.T) {
	p, s := poolPulito(t)
	ctx := context.Background()
	passi := passiScena(t, s.sc)
	fotoPrima := testutil.FotoDelDatabase(t, p)

	var bozza *uuid.UUID
	versione := 0
	for _, ps := range passi {
		r, err := s.salva(ctx, p, s.prodotto, bozza, versione, ps.snap, ps.azioni)
		if err != nil {
			t.Fatalf("%s: %v", ps.nome, err)
		}
		// G05: anche l'ultimo passo (stesso snapshot, con le azioni annulla e rifai) scrive una versione nuova
		if !r.NuovaVersione || r.Versione != versione+1 || r.Eventi != len(ps.azioni) {
			t.Fatalf("%s: %+v", ps.nome, r)
		}
		id := r.BozzaID
		bozza, versione = &id, r.Versione
	}
	// un salvataggio senza modifiche e senza azioni non crea una versione
	r, err := s.salva(ctx, p, s.prodotto, bozza, versione, passi[len(passi)-1].snap, nil)
	if err != nil || r.NuovaVersione || r.Versione != versione || r.SalvataIl.IsZero() {
		t.Fatalf("salvataggio senza modifiche: %+v %v", r, err)
	}
	// salvare tocca solo le tabelle della bozza: niente componenti, archi, proposte, documenti, job, contatori
	diff := testutil.Differenze(fotoPrima, testutil.FotoDelDatabase(t, p))
	for _, tab := range diff {
		if !strings.HasPrefix(tab, "distinta_bozza") {
			t.Errorf("il salvataggio della bozza ha cambiato %s", tab)
		}
	}
	// le tre della bozza piu' distinta_bozza_registro (il passo «aperta», scritto dal trigger: P-04a, direttiva 11)
	if len(diff) != 4 {
		t.Errorf("tabelle cambiate %v, attese le tre della bozza e il suo registro", diff)
	}

	// «riavvio del server»: il pool si chiude, se ne apre uno nuovo, e si rilegge tutto da li'
	p.Close()
	p2 := testutil.Pool(t)
	q := db.New(p2)
	l, err := Leggi(ctx, q, s.thread, s.prodotto)
	if err != nil || l.Stato != StatoAperta || l.BozzaLetta == nil {
		t.Fatalf("Leggi dopo il riavvio: %+v %v", l, err)
	}
	atteso, _ := passi[len(passi)-1].snap.Canonico()
	if !bytes.Equal(l.Snapshot, atteso) {
		t.Fatalf("lo snapshot riletto non e' identico a quello salvato:\n%s\n%s", l.Snapshot, atteso)
	}
	riletto, err := LeggiSnapshot(l.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(riletto.Nodi) != 37 || len(riletto.Archi) != 33 || len(riletto.Decisioni) != 5 {
		t.Errorf("riletti %d nodi, %d archi, %d decisioni: attesi 37, 33, 5", len(riletto.Nodi), len(riletto.Archi), len(riletto.Decisioni))
	}
	esclusi, ricodificati := 0, map[string]string{}
	for _, n := range riletto.Nodi {
		if n.Op == OpEsclude {
			esclusi++
		}
		if n.Op == OpModifica {
			ricodificati[n.ChiaveBozza] = n.Codice
		}
	}
	if esclusi != 3 || ricodificati["k-p1-1"] != "ACM-1101-B" || ricodificati["k-p4-2"] != "ACM-1402-X" {
		t.Errorf("esclusi %d, ricodificati %v", esclusi, ricodificati)
	}
	if arcoDi(riletto, "k-s4", "k-p3-2").Qta != 5 || arcoDi(riletto, "k-s2", "k-p2-3").Qta != 12 || arcoDi(riletto, "k-s2", "k-new2").Posizione != "40" {
		t.Error("spostamenti, qta o posizione non ripristinati")
	}
	if nodoDi(riletto, "k-new2").Codice != "" || len(nodoDi(riletto, "k-s3").Caselle) != 1 || riletto.Sorgente == nil || *riletto.Sorgente.AllegatoID != s.allegato {
		t.Error("il nodo senza codice, le caselle STEP o la fonte non sono ripristinati")
	}
	if l.Versione != len(passi) || l.Base.Firma != firmaProva || l.AggiornataDa != s.utente || l.Fonte == nil || *l.Fonte.AllegatoID != s.allegato {
		t.Errorf("testata riletta: versione %d, base %+v, fonte %+v", l.Versione, l.Base, l.Fonte)
	}
	// il giornale: R1 ne da' il numero, LeggiEventi l'elenco a pagine; ogni azione dichiarata, nell'ordine
	var attese []string
	for i, ps := range passi {
		for _, a := range ps.azioni {
			attese = append(attese, fmt.Sprintf("v%d %s %s", i+1, a.Azione, a.Origine))
		}
	}
	if l.Eventi != len(attese) {
		t.Errorf("R1 dice %d eventi, attesi %d", l.Eventi, len(attese))
	}
	var tutti []EventoLetto
	for dopo := 0; ; {
		pag, err := LeggiEventi(ctx, q, s.thread, *bozza, dopo, 7)
		if err != nil {
			t.Fatal(err)
		}
		if len(pag) == 0 {
			break
		}
		if len(pag) > 7 {
			t.Fatalf("pagina di %d eventi con il limite 7", len(pag))
		}
		tutti = append(tutti, pag...)
		dopo = pag[len(pag)-1].N
	}
	var lette []string
	for i, e := range tutti {
		if e.N != i+1 || e.Vocabolario != Vocabolario || e.UtenteID != s.utente || e.Quando.IsZero() {
			t.Errorf("evento %d: %+v", i, e)
		}
		lette = append(lette, fmt.Sprintf("v%d %s %s", e.Versione, e.Azione, e.Origine))
	}
	if strings.Join(lette, "\n") != strings.Join(attese, "\n") {
		t.Errorf("giornale riletto:\n%s\natteso:\n%s", strings.Join(lette, "\n"), strings.Join(attese, "\n"))
	}
	motivi := 0
	for _, e := range tutti {
		if e.Azione == AzNodoModificato && e.Motivo == "codice letto male dal cartiglio" && e.ChiaveBozza != "" {
			motivi++
		}
	}
	if motivi != 2 {
		t.Errorf("le due ricodifiche rilette con motivo e chiave: %d", motivi)
	}
	// la storia intera si rifa' dal database: eventi e snapshot non divergono (annulla e rifai della versione 6
	// nominano un evento della versione 5, che il rifacimento rilegge dal giornale)
	st, err := LeggiStoria(ctx, q, s.thread, *bozza)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Versioni) != len(passi) {
		t.Fatalf("versioni rilette %d", len(st.Versioni))
	}
	for i, v := range st.Versioni {
		c1, _ := v.Snapshot.Canonico()
		c2, _ := passi[i].snap.Canonico()
		if !bytes.Equal(c1, c2) {
			t.Errorf("la versione %d riletta non e' quella salvata", v.Versione)
		}
	}
	if err := VerificaGiornale(st); err != nil {
		t.Fatalf("il giornale riletto non rifa' le versioni: %v", err)
	}
	// con un evento in meno il rifacimento si ferma
	senzaUno := st
	senzaUno.Eventi = append([]EventoLetto(nil), st.Eventi[:len(st.Eventi)-1]...)
	if err := VerificaGiornale(senzaUno); err == nil {
		t.Error("un giornale senza l'ultimo evento si rifa' lo stesso")
	}
}

// ------------------------------------------------------------------ i conflitti e i rifiuti

func TestIConflittiEIRifiutiDellaBozza(t *testing.T) {
	p, s := poolPulito(t)
	ctx := context.Background()
	passi := passiScena(t, s.sc)
	primo, err := s.salva(ctx, p, s.prodotto, nil, 0, passi[0].snap, passi[0].azioni)
	if err != nil {
		t.Fatal(err)
	}
	b := primo.BozzaID

	// seconda bozza aperta per lo stesso prodotto: 409 BOZZA_VERSIONE, motivo altra_bozza_aperta, con quale
	_, err = s.salva(ctx, p, s.prodotto, nil, 0, passi[0].snap, passi[0].azioni)
	var e *Errore
	if !errors.Is(err, ErrAltraBozzaAperta) || !errors.As(err, &e) || e.Conflitto == nil || e.Conflitto.BozzaID != b || !e.EConflitto() {
		t.Errorf("seconda bozza aperta: %v", err)
	}
	// versione vecchia: 409 BOZZA_VERSIONE versione_diversa, la bozza resta com'e'
	if _, err := s.salva(ctx, p, s.prodotto, &b, 1, passi[1].snap, passi[1].azioni); err != nil {
		t.Fatal(err)
	}
	_, err = s.salva(ctx, p, s.prodotto, &b, 1, passi[1].snap, passi[1].azioni)
	if !errors.Is(err, ErrConflittoVersione) || !errors.As(err, &e) || e.Motivo != MotivoVersioneDiversa || e.Conflitto.Versione != 2 ||
		e.Conflitto.AggiornataDa != s.utente || e.Conflitto.AggiornataIl.IsZero() {
		t.Errorf("salvataggio sulla versione vecchia: %v", err)
	}
	// azioni che non tornano: 422 CORPO_NON_VALIDO / azioni_non_riconciliate, nessuna versione nuova
	if _, err := s.salva(ctx, p, s.prodotto, &b, 2, passi[2].snap, nil); !errors.Is(err, ErrAzioniNonRiconciliate) {
		t.Errorf("snapshot cambiato senza azioni: %v", err)
	}
	// due salvataggi simultanei sulla stessa versione: uno passa, l'altro e' un conflitto
	var wg sync.WaitGroup
	esiti := make([]error, 2)
	for i := range esiti {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, esiti[i] = s.salva(ctx, p, s.prodotto, &b, 2, passi[2].snap, passi[2].azioni)
		}(i)
	}
	wg.Wait()
	ok, conflitti := 0, 0
	for _, err := range esiti {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, ErrConflittoVersione):
			conflitti++
		default:
			t.Errorf("salvataggio simultaneo: %v", err)
		}
	}
	if ok != 1 || conflitti != 1 {
		t.Errorf("salvataggi simultanei: %d riusciti, %d conflitti (attesi 1 e 1)", ok, conflitti)
	}
	lb, _ := LeggiBozza(ctx, db.New(p), s.thread, b)
	if lb.Versione != 3 {
		t.Errorf("dopo i due salvataggi simultanei la bozza e' alla versione %d", lb.Versione)
	}

	// la fonte: la bozza ha f1 (allegato della RFQ, sha b). Cambiarla verso lo sha sbagliato → 422; verso un allegato
	// di un'altra RFQ → FUORI_RFQ; e anche solo di passaggio (f1 → T2 → f1, RC-05) → FUORI_RFQ
	f1 := fonteScena()
	conFonte := func(f Sorgente) Snapshot {
		sn := passi[2].snap.copia()
		sn.Sorgente = &f
		return sn
	}
	cambia := func(da, a Sorgente, motivo string) Azione {
		return Azione{Azione: AzSorgenteCambiata, Origine: "utente", Motivo: motivo, Prima: grezzo(t, da), Dopo: grezzo(t, a)}
	}
	shaSbagliato := Sorgente{AllegatoID: pu(s.allegato), Sha256: shaProva('c'), Gruppo: "cand:a:ACM-1000"}
	if _, err := s.salva(ctx, p, s.prodotto, &b, 3, conFonte(shaSbagliato), []Azione{cambia(f1, shaSbagliato, "prova")}); !errors.Is(err, ErrCorpoNonValido) {
		t.Errorf("fonte con lo sha di un altro file: %v", err)
	}
	t2 := Sorgente{AllegatoID: pu(s.allegatoT2), Sha256: shaProva('e'), Gruppo: "cand:a:ACM-7000"}
	if _, err := s.salva(ctx, p, s.prodotto, &b, 3, conFonte(t2), []Azione{cambia(f1, t2, "prova")}); !errors.Is(err, ErrFuoriRFQ) {
		t.Errorf("fonte di un'altra RFQ: %v", err)
	}
	if _, err := s.salva(ctx, p, s.prodotto, &b, 3, conFonte(f1), []Azione{cambia(f1, t2, "andata"), cambia(t2, f1, "ritorno")}); !errors.Is(err, ErrFuoriRFQ) {
		t.Errorf("fonte di un'altra RFQ solo di passaggio (RC-05): %v", err)
	}
	// cambio di fonte: senza motivo 422; con motivo si', con il rifiuto della prima e una fonte vista per un
	// sottoassieme (destinazione temporanea per chiave, E2.2); la testata segue la fonte
	f2 := Sorgente{AllegatoID: pu(s.allegato2), Sha256: shaProva('c'), Gruppo: "cand:a2:ACM-1000"}
	cambio := []Azione{
		{Azione: AzSorgenteRifiutata, Origine: "utente", Motivo: "manca il telaio", Dopo: grezzo(t, map[string]any{"allegato_id": s.allegato, "gruppo": f1.Gruppo})},
		{Azione: AzSorgenteVista, ChiaveBozza: "k-new1", Origine: "utente", Dopo: grezzo(t, map[string]any{"allegato_id": s.allegato2, "gruppo": "cand:a2:ACM-9001", "sha256": shaProva('c')})},
		cambia(f1, f2, ""),
	}
	if _, err := s.salva(ctx, p, s.prodotto, &b, 3, conFonte(f2), cambio); !errors.Is(err, ErrCorpoNonValido) {
		t.Errorf("cambio di fonte senza motivo: %v", err)
	}
	cambio[2].Motivo = "la prima candidata era incompleta"
	if _, err := s.salva(ctx, p, s.prodotto, &b, 3, conFonte(f2), cambio); err != nil {
		t.Fatalf("cambio di fonte con motivo: %v", err)
	}
	testata, _ := db.New(p).GetDistintaBozza(ctx, db.GetDistintaBozzaParams{BozzaID: b, ThreadID: s.thread})
	if testata.FonteAllegatoID.UUID != s.allegato2 || strings.TrimSpace(testata.FonteSha256.String) != shaProva('c') || testata.Versione != 4 {
		t.Errorf("la testata non segue la fonte nuova: %+v", testata)
	}
	ee, _ := LeggiEventi(ctx, db.New(p), s.thread, b, 0, 0)
	if vista := ee[len(ee)-2]; vista.Azione != AzSorgenteVista || vista.ChiaveBozza != "k-new1" {
		t.Errorf("la fonte vista per un sottoassieme non porta la destinazione: %+v", vista)
	}
	// G05: lo stesso snapshot con una sola azione informativa scrive una versione nuova (gli eventi vogliono una
	// versione); senza azioni no
	vista := []Azione{{Azione: AzSorgenteVista, Origine: "utente", Dopo: grezzo(t, map[string]any{"allegato_id": s.allegato, "gruppo": f1.Gruppo})}}
	if r, err := s.salva(ctx, p, s.prodotto, &b, 4, conFonte(f2), vista); err != nil || !r.NuovaVersione || r.Versione != 5 || r.Eventi != 1 {
		t.Errorf("stesso snapshot con un'azione: %+v %v", r, err)
	}
	if r, err := s.salva(ctx, p, s.prodotto, &b, 5, conFonte(f2), nil); err != nil || r.NuovaVersione || r.Versione != 5 {
		t.Errorf("stesso snapshot senza azioni: %+v %v", r, err)
	}
	sn := conFonte(f2)

	// appartenenza: prodotto di un'altra RFQ, archiviato, sottoassieme; bozza di un'altra RFQ; RFQ che non c'e'
	for nome, prod := range map[string]uuid.UUID{"di un'altra RFQ": s.prodottoT2, "archiviato": s.archiviato, "sottoassieme": s.sotto} {
		if _, err := s.salva(ctx, p, prod, nil, 0, soloRadice(prod, "ACM-X"), []Azione{apertura(t, PartenzaVuota)}); !errors.Is(err, ErrFuoriRFQ) {
			t.Errorf("prodotto %s: %v", nome, err)
		}
	}
	if _, err := Salva(ctx, p, Salvataggio{ThreadID: s.thread2, ProdottoID: s.prodotto, UtenteID: s.utente, BozzaID: &b, VersioneAttesa: 5,
		Vocabolario: 1, Snapshot: grezzo(t, sn)}); !errors.Is(err, ErrFuoriRFQ) {
		t.Errorf("bozza salvata da un'altra RFQ: %v", err)
	}
	if _, err := Salva(ctx, p, Salvataggio{ThreadID: uuid.New(), ProdottoID: s.prodotto, UtenteID: s.utente, Vocabolario: 1,
		Snapshot: grezzo(t, passi[0].snap), Azioni: passi[0].azioni}); !errors.Is(err, ErrRFQInesistente) {
		t.Errorf("RFQ che non c'e': %v", err)
	}
	// G02: la conferma della base deve essere una conferma di questo prodotto della RFQ
	conBase := soloRadice(s.altroFinito, "ACM-6000")
	conBase.VersioneBomBase = &VersioneBomBase{ConfermaID: pu(uuid.New()), Numero: 1, Contatore: 0}
	if _, err := s.salva(ctx, p, s.altroFinito, nil, 0, conBase, []Azione{apertura(t, PartenzaVuota)}); !errors.Is(err, ErrFuoriRFQ) {
		t.Errorf("base con una conferma che non e' del prodotto (G02): %v", err)
	}

	// abbandono: versione vecchia 409; giusta → chiusa, R1 dice nessuna_bozza, un salvataggio dopo e' un conflitto
	if _, err := Abbandona(ctx, p, Abbandono{ThreadID: s.thread, BozzaID: b, UtenteID: s.utente, VersioneAttesa: 4}); !errors.Is(err, ErrConflittoVersione) {
		t.Errorf("abbandono sulla versione vecchia: %v", err)
	}
	if _, err := Abbandona(ctx, p, Abbandono{ThreadID: s.thread2, BozzaID: b, UtenteID: s.utente, VersioneAttesa: 5}); !errors.Is(err, ErrFuoriRFQ) {
		t.Errorf("abbandono da un'altra RFQ: %v", err)
	}
	if _, err := Abbandona(ctx, p, Abbandono{ThreadID: s.thread, BozzaID: b, UtenteID: s.utente, VersioneAttesa: 5, Motivo: "riga\x00nulla"}); !errors.Is(err, ErrCorpoNonValido) {
		t.Errorf("abbandono con un motivo con U+0000 (RC-07): %v", err)
	}
	ab, err := Abbandona(ctx, p, Abbandono{ThreadID: s.thread, BozzaID: b, UtenteID: s.utente, VersioneAttesa: 5, Motivo: "  si ricomincia dalla BOM  "})
	if err != nil || ab.ChiusaIl.IsZero() || ab.Versione != 5 || ab.Stato != StatoAbbandonata {
		t.Fatalf("abbandono: %+v %v", ab, err)
	}
	if l, err := Leggi(ctx, db.New(p), s.thread, s.prodotto); err != nil || l.Stato != StatoNessunaBozza || l.BozzaLetta != nil {
		t.Errorf("R1 dopo l'abbandono: %+v %v", l, err)
	}
	chiusa, err := LeggiBozza(ctx, db.New(p), s.thread, b)
	if err != nil || chiusa.Stato != StatoAbbandonata || chiusa.MotivoChiusura != "si ricomincia dalla BOM" || chiusa.Eventi == 0 {
		t.Errorf("bozza abbandonata riletta: %+v %v", chiusa.Stato, err)
	}
	_, err = Salva(ctx, p, Salvataggio{ThreadID: s.thread, ProdottoID: s.prodotto, UtenteID: s.utente, BozzaID: &b, VersioneAttesa: 5,
		Vocabolario: 1, Snapshot: grezzo(t, sn)})
	if !errors.Is(err, ErrConflittoVersione) || !errors.As(err, &e) || e.Motivo != MotivoBozzaChiusa || e.Conflitto.Stato != StatoAbbandonata {
		t.Errorf("salvataggio su una bozza abbandonata: %v", err)
	}
	// dopo l'abbandono si apre una bozza nuova dello stesso prodotto
	if r, err := s.salva(ctx, p, s.prodotto, nil, 0, passi[0].snap, passi[0].azioni); err != nil || r.BozzaID == b {
		t.Errorf("bozza nuova dopo l'abbandono: %+v %v", r, err)
	}
}

// RC-02: la prima versione si confronta con lo stato di partenza ricostruito dal database.
func TestLaPrimaVersioneSiConfrontaConIlDatabase(t *testing.T) {
	p, s := poolPulito(t)
	ctx := context.Background()
	// un nodo «step» inventato senza azione, partendo dal registrato: no
	inventato := s.sc.snap.copia()
	inventato.Nodi = append(inventato.Nodi, Nodo{ChiaveBozza: "k-fantasma", Op: OpCrea, Codice: "ACM-F", Tipo: "sciolto", Origine: "step"})
	if _, err := s.salva(ctx, p, s.prodotto, nil, 0, inventato, []Azione{apertura(t, PartenzaRegistrato)}); !errors.Is(err, ErrAzioniNonRiconciliate) {
		t.Errorf("nodo inventato nella prima versione: %v", err)
	}
	// la working cambia sotto la pagina: l'arco s1 → p1-2 con qta 9 nel database; lo snapshot dice 2
	if _, err := p.Exec(ctx, `UPDATE componente_relazione SET qta = 9 WHERE padre_id = $1 AND figlio_id = $2`, s.sc.componenti["k-s1"], s.sc.componenti["k-p1-2"]); err != nil {
		t.Fatal(err)
	}
	if _, err := s.salva(ctx, p, s.prodotto, nil, 0, s.sc.snap, []Azione{apertura(t, PartenzaRegistrato)}); !errors.Is(err, ErrAzioniNonRiconciliate) {
		t.Errorf("partenza con una qta diversa dalla working: %v", err)
	}
	// partenza vuota con la radice sbagliata (codice diverso dal registrato): no
	if _, err := s.salva(ctx, p, s.altroFinito, nil, 0, soloRadice(s.altroFinito, "ACM-ALTRO"), []Azione{apertura(t, PartenzaVuota)}); !errors.Is(err, ErrAzioniNonRiconciliate) {
		t.Errorf("radice diversa dal registrato: %v", err)
	}
	// partenze che B1 non ricostruisce: 422 partenza_non_ricostruibile, mai accettate senza confronto
	for _, pt := range []string{PartenzaStep, PartenzaConferma} {
		if _, err := s.salva(ctx, p, s.altroFinito, nil, 0, soloRadice(s.altroFinito, "ACM-6000"), []Azione{apertura(t, pt)}); !errors.Is(err, ErrPartenzaNonRicostruibile) {
			t.Errorf("partenza %s: %v", pt, err)
		}
	}
	if l, err := Leggi(ctx, db.New(p), s.thread, s.altroFinito); err != nil || l.Stato != StatoNessunaBozza {
		t.Errorf("dopo i rifiuti c'e' una bozza: %+v %v", l, err)
	}
}

// R-11 / RD-12 (revisioni della notte): un arco rimasto nella working verso un componente archiviato non porta
// l'archiviato nella partenza «registrato» (come v_componente_albero e il perimetro chiuso della conferma, che non lo
// certifica): il perimetro e i suoi archi lo escludono, e la prima versione dello snapshot registrato si salva.
func TestLaPartenzaRegistratoNonPortaGliArchiviati(t *testing.T) {
	p, s := poolPulito(t)
	ctx := context.Background()
	// l'arco scritto a mano (ArchiviaComponente toglie gli archi prima di archiviare: qui un arco rimasto)
	if _, err := p.Exec(ctx, `INSERT INTO componente_relazione (thread_id, padre_id, figlio_id, qta, origine, confermato_da) VALUES ($1, $2, $3, 1, 'manuale', $4)`,
		s.thread, s.prodotto, s.archiviato, s.utente); err != nil {
		t.Fatal(err)
	}
	q := db.New(p)
	perimetro, err := q.ListPerimetroRegistrato(ctx, db.ListPerimetroRegistratoParams{ThreadID: s.thread, ProdottoID: s.prodotto})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range perimetro {
		if c.ComponenteID == s.archiviato || c.ArchiviatoIl != nil {
			t.Errorf("il perimetro registrato contiene un archiviato: %+v", c)
		}
	}
	if len(perimetro) != len(s.sc.snap.Nodi) {
		t.Errorf("perimetro registrato: %d componenti, attesi %d", len(perimetro), len(s.sc.snap.Nodi))
	}
	archi, err := q.ListArchiPerimetroRegistrato(ctx, db.ListArchiPerimetroRegistratoParams{ThreadID: s.thread, ProdottoID: s.prodotto})
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range archi {
		if a.FiglioID == s.archiviato {
			t.Errorf("gli archi del perimetro registrato contengono l'arco verso l'archiviato: %+v", a)
		}
	}
	if len(archi) != len(s.sc.snap.Archi) {
		t.Errorf("archi del perimetro registrato: %d, attesi %d", len(archi), len(s.sc.snap.Archi))
	}
	if _, err := s.salva(ctx, p, s.prodotto, nil, 0, s.sc.snap, []Azione{apertura(t, PartenzaRegistrato)}); err != nil {
		t.Errorf("prima versione registrato con un arco rimasto verso un archiviato: %v", err)
	}
}

// E2.3 e E2.12: una bozza con un nodo op=crea senza codice (la posizione del cartiglio sull'arco) e con una
// dichiarazione mono_pezzo non applicabile (il prodotto ha un figlio) si salva, si rilegge identica, e si salva
// ancora.
func TestUnNodoSenzaCodiceEUnMonoPezzoNonApplicabileSiSalvanoESiRileggono(t *testing.T) {
	p, s := poolPulito(t)
	ctx := context.Background()
	sn := soloRadice(s.altroFinito, "ACM-6000")
	x := Nodo{ChiaveBozza: "k-staffa", Op: OpCrea, Descrizione: "Staffa del cartiglio", Tipo: "sciolto", Origine: "utente"}
	a := Arco{Padre: "r", Figlio: "k-staffa", Qta: 2, Origine: "utente", Posizione: "12"}
	mono := map[string]any{"id": "d-mono", "tipo": "mono_pezzo", "motivo": "pezzo unico tornito dal pieno"}
	sn.Nodi = append(sn.Nodi, x)
	sn.Archi = append(sn.Archi, a)
	sn.Decisioni = append(sn.Decisioni, canonico(mono))
	r, err := s.salva(ctx, p, s.altroFinito, nil, 0, sn, []Azione{apertura(t, PartenzaVuota),
		{Azione: AzNodoAggiunto, Origine: "utente", Dopo: grezzo(t, x)},
		{Azione: AzArcoAggiunto, Origine: "utente", Dopo: grezzo(t, a)},
		{Azione: AzDecisioneAggiunta, Origine: "utente", Dopo: grezzo(t, mono)}})
	if err != nil {
		t.Fatalf("bozza con il nodo senza codice: %v", err)
	}
	l, err := Leggi(ctx, db.New(p), s.thread, s.altroFinito)
	if err != nil {
		t.Fatal(err)
	}
	atteso, _ := sn.Canonico()
	if !bytes.Equal(l.Snapshot, atteso) || l.Impronta != r.Impronta {
		t.Fatalf("la bozza non si rilegge identica:\n%s\n%s", l.Snapshot, atteso)
	}
	if bytes.Contains(l.Snapshot, []byte(`"chiave_bozza":"k-staffa","codice"`)) || !bytes.Contains(l.Snapshot, []byte(`"posizione":"12"`)) {
		t.Errorf("il codice e' stato inventato o la posizione persa: %s", l.Snapshot)
	}
	// poi il codice si assegna con nodo_modificato
	conCodice := sn.copia()
	cambiaNodo(&conCodice, "k-staffa", func(n *Nodo) { n.Codice = "ACM-6012" })
	if _, err := s.salva(ctx, p, s.altroFinito, &r.BozzaID, 1, conCodice, []Azione{{Azione: AzNodoModificato, ChiaveBozza: "k-staffa", Origine: "utente",
		Prima: grezzo(t, map[string]any{"codice": nil}), Dopo: grezzo(t, map[string]string{"codice": "ACM-6012"})}}); err != nil {
		t.Errorf("assegnazione del codice: %v", err)
	}
}

// RC-01 e RC-07: i numeri e i caratteri che jsonb riscrive o rifiuta non entrano; i pesi oltre i CHECK della 0022 sono
// un CORPO_NON_VALIDO con il campo, non un errore grezzo; una bozza salvata si rilegge e si risalva sempre.
func TestINumeriIPesiEICaratteriCheIlDatabaseNonVuole(t *testing.T) {
	p, s := poolPulito(t)
	ctx := context.Background()
	base := soloRadice(s.altroFinito, "ACM-6000")
	r, err := s.salva(ctx, p, s.altroFinito, nil, 0, base, []Azione{apertura(t, PartenzaVuota)})
	if err != nil {
		t.Fatal(err)
	}
	b := r.BozzaID
	conDecisione := func(d json.RawMessage) (Snapshot, []Azione) {
		sn := base.copia()
		sn.Decisioni = []json.RawMessage{d}
		return sn, []Azione{{Azione: AzDecisioneAggiunta, Origine: "utente", Dopo: d}}
	}
	// RC-01: 1e2 e -0 si rifiutano prima del database
	for _, num := range []string{"1e2", "-0"} {
		sn, az := conDecisione(json.RawMessage(`{"id":"d-n","tipo":"arco_condiviso","qta":` + num + `}`))
		if _, err := s.salva(ctx, p, s.altroFinito, &b, 1, sn, az); !errors.Is(err, ErrCorpoNonValido) {
			t.Errorf("numero %s in una decisione: %v", num, err)
		}
	}
	// RC-07: U+0000 in una decisione: CORPO_NON_VALIDO, non l'errore 22P05 di PostgreSQL
	sn, az := conDecisione(json.RawMessage(`{"id":"d-nul","tipo":"mono_pezzo","motivo":"a\u0000b"}`))
	if _, err := s.salva(ctx, p, s.altroFinito, &b, 1, sn, az); !errors.Is(err, ErrCorpoNonValido) {
		t.Errorf("U+0000 in una decisione: %v", err)
	}
	// un intero grande e una stringa in NFD si salvano, si rileggono alla stessa impronta, e la bozza si risalva
	sn, az = conDecisione(json.RawMessage(`{"id":"d-ok","tipo":"arco_condiviso","qta":12345678901234567890123,"nota":"è"}`))
	r, err = s.salva(ctx, p, s.altroFinito, &b, 1, sn, az)
	if err != nil {
		t.Fatalf("decisione con un intero grande: %v", err)
	}
	l, err := Leggi(ctx, db.New(p), s.thread, s.altroFinito)
	if err != nil || l.Impronta != r.Impronta {
		t.Fatalf("rilettura dopo un intero grande: %+v %v", l.Impronta, err)
	}
	if !bytes.Contains(l.Snapshot, []byte("12345678901234567890123")) || !bytes.Contains(l.Snapshot, []byte("\"nota\":\"è\"")) {
		t.Errorf("snapshot riletto: %s", l.Snapshot)
	}
	if _, err := s.salva(ctx, p, s.altroFinito, &b, 2, sn, []Azione{{Azione: AzSorgenteVista, Origine: "utente",
		Dopo: grezzo(t, map[string]any{"allegato_id": s.allegato, "gruppo": "bom"})}}); err != nil {
		t.Errorf("la bozza non si risalva dopo la rilettura (RC-01): %v", err)
	}
	// RC-07: una decisione di 120 KB di testo (sotto MaxPrimaDopo) pesa 720 KB come jsonb: il CHECK ck_dbe_dopo
	lista := `[` + strings.Repeat("1,", 59999) + `1]`
	grande := json.RawMessage(`{"id":"d-grande","tipo":"da_valutare","l":` + lista + `}`)
	sn2 := sn.copia()
	sn2.Decisioni = append(sn2.Decisioni, grande)
	_, err = s.salva(ctx, p, s.altroFinito, &b, 3, sn2, []Azione{{Azione: AzDecisioneAggiunta, Origine: "utente", Dopo: grande}})
	var e *Errore
	if !errors.As(err, &e) || e.Codice != CodiceCorpoNonValido || e.Campo != "azioni[0].dopo" {
		t.Errorf("dopo oltre 256 KiB di jsonb: %v", err)
	}
	// RC-07: venti decisioni da 42 KB di testo (252 KB di jsonb l'una, sotto il CHECK degli eventi) fanno uno
	// snapshot sotto i 2 MiB di testo ma oltre i 4 MiB di jsonb: il CHECK ck_dbv_snapshot
	sn3 := sn.copia()
	var molte []Azione
	medio := `[` + strings.Repeat("1,", 20999) + `1]`
	for i := 0; i < 20; i++ {
		d := json.RawMessage(fmt.Sprintf(`{"id":"d-m%02d","tipo":"da_valutare","l":%s}`, i, medio))
		sn3.Decisioni = append(sn3.Decisioni, d)
		molte = append(molte, Azione{Azione: AzDecisioneAggiunta, Origine: "utente", Dopo: d})
	}
	_, err = s.salva(ctx, p, s.altroFinito, &b, 3, sn3, molte)
	if !errors.As(err, &e) || e.Codice != CodiceCorpoNonValido || e.Campo != "snapshot" {
		t.Errorf("snapshot oltre 4 MiB di jsonb: %v", err)
	}
	// dopo i rifiuti la bozza e' ancora alla versione 3 e si rilegge
	if l, err := Leggi(ctx, db.New(p), s.thread, s.altroFinito); err != nil || l.Versione != 3 {
		t.Errorf("dopo i rifiuti: %+v %v", l.Versione, err)
	}
}

// RC-09: R1 controlla il perimetro come R2.
func TestLeggiControllaIlPerimetro(t *testing.T) {
	p, s := poolPulito(t)
	ctx := context.Background()
	q := db.New(p)
	if _, err := Leggi(ctx, q, uuid.New(), s.prodotto); !errors.Is(err, ErrRFQInesistente) {
		t.Errorf("RFQ che non c'e': %v", err)
	}
	for nome, prod := range map[string]uuid.UUID{"di un'altra RFQ": s.prodottoT2, "sottoassieme": s.sotto, "che non c'e'": uuid.New()} {
		if _, err := Leggi(ctx, q, s.thread, prod); !errors.Is(err, ErrFuoriRFQ) {
			t.Errorf("prodotto %s: %v", nome, err)
		}
	}
	if l, err := Leggi(ctx, q, s.thread, s.archiviato); err != nil || l.Stato != StatoNessunaBozza {
		t.Errorf("un finito archiviato si legge: %+v %v", l, err)
	}
	if l, err := Leggi(ctx, q, s.thread, s.prodotto); err != nil || l.Stato != StatoNessunaBozza {
		t.Errorf("nessuna bozza: %+v %v", l, err)
	}
	if _, err := LeggiEventi(ctx, q, s.thread, uuid.New(), 0, 10); !errors.Is(err, ErrFuoriRFQ) {
		t.Errorf("eventi di una bozza che non c'e': %v", err)
	}
}

// Il rebase (contratto rev. 3 §6.7): azione e campo «rebase» insieme, motivo, base corrente (BASE_SUPERATA), e la
// conferma della base di un altro prodotto (FUORI_RFQ).
func TestIlRebaseVuoleLaBaseCorrente(t *testing.T) {
	p, s := poolPulito(t)
	ctx := context.Background()
	passi := passiScena(t, s.sc)
	r, err := s.salva(ctx, p, s.prodotto, nil, 0, passi[0].snap, passi[0].azioni)
	if err != nil {
		t.Fatal(err)
	}
	b := r.BozzaID
	wc, err := db.New(p).GetWorkingContatore(ctx, s.thread)
	if err != nil {
		t.Fatal(err)
	}
	prima := baseDelloSnapshot(passi[0].snap)
	f := fonteScena()
	conBase := func(nb Base) (Snapshot, []Azione, *Rebase) {
		sn := passi[0].snap.copia()
		sn.VersioneBomBase, sn.FirmaBase = nb.versione(), nb.Firma
		az := []Azione{{Azione: AzRebase, Origine: "utente", Motivo: "la working e' cambiata",
			Prima: grezzo(t, map[string]any{"base": prima, "sorgente": f}),
			Dopo:  grezzo(t, map[string]any{"base": nb, "sorgente": f, "rif_scaduti": []string{}})}}
		return sn, az, &Rebase{Base: nb, Sorgente: &f, Motivo: "la working e' cambiata"}
	}
	salvaR := func(sn Snapshot, az []Azione, rb *Rebase) error {
		_, err := Salva(ctx, p, Salvataggio{ThreadID: s.thread, ProdottoID: s.prodotto, UtenteID: s.utente, BozzaID: &b, VersioneAttesa: 1,
			Vocabolario: 1, Snapshot: grezzo(t, sn), Azioni: az, Rebase: rb})
		return err
	}
	// contatore vecchio: 409 BASE_SUPERATA struttura_cambiata
	sn, az, rb := conBase(Base{Numero: 0, Contatore: wc.Struttura + 7, Firma: shaProva('d')})
	err = salvaR(sn, az, rb)
	var e *Errore
	if !errors.Is(err, ErrBaseSuperata) || !errors.As(err, &e) || len(e.Motivi) != 1 || e.Motivi[0] != MotivoStrutturaCambiata || !e.EConflitto() {
		t.Errorf("rebase con il contatore vecchio: %v", err)
	}
	// una conferma che non e' del prodotto: FUORI_RFQ
	sn, az, rb = conBase(Base{ConfermaID: pu(uuid.New()), Numero: 1, Contatore: wc.Struttura, Firma: shaProva('d')})
	if err := salvaR(sn, az, rb); !errors.Is(err, ErrFuoriRFQ) {
		t.Errorf("rebase su una conferma di nessuno: %v", err)
	}
	// azione senza il campo rebase, o campo con un'altra base: 422 campo rebase
	sn, az, rb = conBase(Base{Numero: 0, Contatore: wc.Struttura, Firma: shaProva('d')})
	if err := salvaR(sn, az, nil); !errors.As(err, &e) || e.Campo != "rebase" {
		t.Errorf("rebase senza il campo: %v", err)
	}
	altro := *rb
	altro.Base.Firma = shaProva('9')
	if err := salvaR(sn, az, &altro); !errors.As(err, &e) || e.Campo != "rebase" {
		t.Errorf("campo rebase con un'altra firma: %v", err)
	}
	// base corrente, azione e campo coerenti: si'; la testata segue la base nuova
	if err := salvaR(sn, az, rb); err != nil {
		t.Fatalf("rebase sulla base corrente: %v", err)
	}
	testata, _ := db.New(p).GetDistintaBozza(ctx, db.GetDistintaBozzaParams{BozzaID: b, ThreadID: s.thread})
	if testata.BaseContatore != wc.Struttura || strings.TrimSpace(testata.FirmaBase) != shaProva('d') || testata.Versione != 2 {
		t.Errorf("la testata non segue il rebase: %+v", testata)
	}
}

// D-P4B1-04 A [U] (messaggio 10 del 12/10, «Retention») e D-P4A-01 = P-04a [U] (direttiva 11: registro permanente
// delle bozze). La prova descrive il comportamento della 0022:
//   - una bozza confermata non si cancella mai, e finche' c'e' il prodotto non si rimuove;
//   - una bozza abbandonata si cancella solo oltre i 90 giorni, con versioni ed eventi; finche' c'e' tiene il prodotto;
//   - dopo quella cancellazione il prodotto resta tenuto da distinta_bozza_registro (FK permanente): non si rimuove,
//     si archivia, e il gesto legacy «rimuovi» lo dice con le sue parole.
func TestLaConservazioneDelleBozzeEIlProdotto(t *testing.T) {
	p, s := poolPulito(t)
	ctx := context.Background()
	q := db.New(p)
	// una bozza di altroFinito, abbandonata
	r, err := s.salva(ctx, p, s.altroFinito, nil, 0, soloRadice(s.altroFinito, "ACM-6000"), []Azione{apertura(t, PartenzaVuota)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Abbandona(ctx, p, Abbandono{ThreadID: s.thread, BozzaID: r.BozzaID, UtenteID: s.utente, VersioneAttesa: 1}); err != nil {
		t.Fatal(err)
	}
	// abbandonata da poco: non si cancella, e il prodotto non si rimuove
	if _, err := p.Exec(ctx, `DELETE FROM distinta_bozza WHERE bozza_id = $1`, r.BozzaID); codiceSQL(err) != "P4X04" {
		t.Errorf("cancellazione di una bozza abbandonata da poco: %v", err)
	}
	if _, err := q.DeleteComponente(ctx, s.altroFinito); codiceSQL(err) != "23503" {
		t.Errorf("rimozione del prodotto con una bozza abbandonata: %v", err)
	}
	// il gesto legacy «rimuovi» lo dice con le sue parole (cheCosaLoTiene, nota B1 par.11.6 n. 14)
	if _, err := fascicolo.RimuoviComponente(ctx, q, s.thread, s.altroFinito); err == nil || !strings.Contains(err.Error(), "ha una bozza della distinta") {
		t.Errorf("RimuoviComponente con una bozza: %v", err)
	}
	// abbandonata da 91 giorni (data retrodatata spegnendo il trigger del ciclo nella transazione): la bozza si
	// cancella con versioni ed eventi, e il prodotto resta tenuto dal registro delle bozze
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	for _, sql := range []string{
		`ALTER TABLE distinta_bozza DISABLE TRIGGER trg_distinta_bozza_ciclo`,
		`UPDATE distinta_bozza SET chiusa_il = now() - interval '91 days' WHERE bozza_id = '` + r.BozzaID.String() + `'`,
		`ALTER TABLE distinta_bozza ENABLE TRIGGER trg_distinta_bozza_ciclo`,
	} {
		if _, err := tx.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	tag, err := tx.Exec(ctx, `DELETE FROM distinta_bozza WHERE bozza_id = $1`, r.BozzaID)
	if err != nil || tag.RowsAffected() != 1 {
		t.Fatalf("cancellazione oltre la conservazione: %v", err)
	}
	var versioni, eventi int
	if err := tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM distinta_bozza_versione WHERE bozza_id = $1), (SELECT count(*) FROM distinta_bozza_evento WHERE bozza_id = $1)`,
		r.BozzaID).Scan(&versioni, &eventi); err != nil || versioni != 0 || eventi != 0 {
		t.Errorf("versioni %d ed eventi %d dopo la cancellazione in cascata: %v", versioni, eventi, err)
	}
	var passi string
	if err := tx.QueryRow(ctx, `SELECT string_agg(passo, ' ' ORDER BY registro_id) FROM distinta_bozza_registro WHERE bozza_id = $1`, r.BozzaID).Scan(&passi); err != nil ||
		passi != "aperta abbandonata cancellata" {
		t.Errorf("registro della bozza dopo la conservazione: %q %v", passi, err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := q.DeleteComponente(ctx, s.altroFinito); codiceSQL(err) != "23503" {
		t.Errorf("rimozione del prodotto dopo la conservazione della bozza (P-04a): %v", err)
	}
	if _, err := fascicolo.RimuoviComponente(ctx, q, s.thread, s.altroFinito); err == nil || !strings.Contains(err.Error(), "ha una storia di bozze della distinta") {
		t.Errorf("RimuoviComponente dopo la conservazione della bozza: %v", err)
	}
}

// Una bozza confermata non si cancella mai, nemmeno «oltre la conservazione», e tiene il prodotto (con la conferma).
func TestUnaBozzaConfermataNonSiCancella(t *testing.T) {
	p, s := poolPulito(t)
	ctx := context.Background()
	fin := idDi("finito-confermato")
	if _, err := p.Exec(ctx, `INSERT INTO componente (componente_id, thread_id, codice, tipo, origine, confermato_da) VALUES ($1, $2, 'ACM-6500', 'finito', 'manuale', $3)`,
		fin, s.thread, s.utente); err != nil {
		t.Fatal(err)
	}
	sn := soloRadice(fin, "ACM-6500")
	r, err := s.salva(ctx, p, fin, nil, 0, sn, []Azione{apertura(t, PartenzaVuota)})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	for _, sql := range []string{
		`INSERT INTO conferma_strutturale (thread_id, prodotto_id, numero, formato, senza_riepilogo_v2, firma_struttura, firma_anteprima,
			base_contatore, firma_base, contatore, contatore_dati, bozza_id, bozza_versione, origine, snapshot, impronta_snapshot, delta, mappa,
			criterio_verifica, chiave_idempotenza, confermato_da)
		 SELECT $1::uuid, $2::uuid, 1, 2, false, repeat('3', 64), repeat('2', 64), 0, repeat('1', 64),
			COALESCE((SELECT struttura FROM working_contatore WHERE thread_id = $1), 0), COALESCE((SELECT dati FROM working_contatore WHERE thread_id = $1), 0),
			$3::uuid, 1, '{"rotta":"prova"}', jsonb_build_object('formato', 2, 'prodotto_id', $2::uuid::text),
			(SELECT impronta FROM distinta_bozza_versione WHERE bozza_id = $3::uuid AND versione = 1), '{}', '{"r":"x"}', 'nessuno', gen_random_uuid(), $4::uuid`,
		`INSERT INTO conferma_strutturale_componente (conferma_id, thread_id, componente_id, radice, chiave_bozza, codice, tipo)
		 SELECT conferma_id, $1::uuid, $2::uuid, true, 'r', 'ACM-6500', 'finito' FROM conferma_strutturale WHERE bozza_id = $3::uuid AND $4::uuid IS NOT NULL`,
		`UPDATE distinta_bozza SET stato = 'confermata', chiusa_da = $4::uuid WHERE bozza_id = $3::uuid AND $1::uuid IS NOT NULL AND $2::uuid IS NOT NULL`,
	} {
		if _, err := tx.Exec(ctx, sql, s.thread, fin, r.BozzaID, s.utente); err != nil {
			t.Fatalf("%v: %s", err, sql)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("conferma che chiude la bozza: %v", err)
	}
	lb, err := LeggiBozza(ctx, db.New(p), s.thread, r.BozzaID)
	if err != nil || lb.Stato != StatoConfermata {
		t.Fatalf("bozza confermata riletta: %+v %v", lb.Stato, err)
	}
	// la conferma vera della base: GetConfermaStrutturaleBase la trova, e una base con il numero sbagliato e' un 422
	var conferma uuid.UUID
	if err := p.QueryRow(ctx, `SELECT conferma_id FROM conferma_strutturale WHERE bozza_id = $1`, r.BozzaID).Scan(&conferma); err != nil {
		t.Fatal(err)
	}
	if c, err := db.New(p).GetConfermaStrutturaleBase(ctx, db.GetConfermaStrutturaleBaseParams{ThreadID: s.thread, ConfermaID: conferma}); err != nil || c.Numero != 1 || c.ProdottoID != fin {
		t.Errorf("GetConfermaStrutturaleBase: %+v %v", c, err)
	}
	if err := baseDellaRfq(ctx, db.New(p), s.thread, fin, Base{ConfermaID: &conferma, Numero: 2, Firma: firmaProva}); !errors.Is(err, ErrCorpoNonValido) {
		t.Errorf("base con il numero sbagliato: %v", err)
	}
	if err := baseDellaRfq(ctx, db.New(p), s.thread, s.altroFinito, Base{ConfermaID: &conferma, Numero: 1, Firma: firmaProva}); !errors.Is(err, ErrFuoriRFQ) {
		t.Errorf("base con la conferma di un altro prodotto: %v", err)
	}
	if _, err := p.Exec(ctx, `DELETE FROM distinta_bozza WHERE bozza_id = $1`, r.BozzaID); codiceSQL(err) != "P4X04" {
		t.Errorf("cancellazione di una bozza confermata: %v", err)
	}
	tx, err = p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	for _, sql := range []string{
		`ALTER TABLE distinta_bozza DISABLE TRIGGER trg_distinta_bozza_ciclo`,
		`UPDATE distinta_bozza SET chiusa_il = now() - interval '400 days' WHERE bozza_id = '` + r.BozzaID.String() + `'`,
		`ALTER TABLE distinta_bozza ENABLE TRIGGER trg_distinta_bozza_ciclo`,
	} {
		if _, err := tx.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM distinta_bozza WHERE bozza_id = $1`, r.BozzaID); codiceSQL(err) != "P4X04" {
		t.Errorf("cancellazione di una bozza confermata da 400 giorni: %v", err)
	}
	_ = tx.Rollback(ctx)
	if _, err := db.New(p).DeleteComponente(ctx, fin); codiceSQL(err) != "23503" {
		t.Errorf("rimozione del prodotto con bozza confermata: %v", err)
	}
	if _, err := fascicolo.RimuoviComponente(ctx, db.New(p), s.thread, fin); err == nil || !strings.Contains(err.Error(), "non si cancella") ||
		strings.Contains(err.Error(), "ancora usato") {
		t.Errorf("RimuoviComponente con una conferma: la frase deve dire che cosa lo tiene: %v", err)
	}
	// salvare su una bozza confermata e' un conflitto (bozza_chiusa)
	if _, err := s.salva(ctx, p, fin, &r.BozzaID, 1, sn, nil); !errors.Is(err, ErrConflittoVersione) {
		t.Errorf("salvataggio su una bozza confermata: %v", err)
	}
}

func codiceSQL(err error) string {
	type conCodice interface{ SQLState() string }
	var c conCodice
	if errors.As(err, &c) {
		return c.SQLState()
	}
	return ""
}
