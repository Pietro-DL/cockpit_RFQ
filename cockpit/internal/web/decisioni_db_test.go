//go:build integrazione

// L4 — voce 1.9: due operatori che decidono sullo stesso messaggio nello stesso momento (T13, T14).
//
// Perimetro di questi test, dichiarato subito perché conta: si prova il MECCANISMO di decisione — la
// transazione con il blocco di riga e il percorso di aggancio condiviso da tutti gli handler — non il
// giro HTTP completo con sessione e form. La corsa fra due operatori vive lì: se due transazioni
// possono decidere insieme, nessun controllo scritto sopra la transazione lo impedisce.
package web

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/domain"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/testutil"
)

type scena struct {
	pool     *pgxpool.Pool
	q        *db.Queries
	utenteA  db.Utente
	utenteB  db.Utente
	cliente  db.Cliente
	messagio uuid.UUID
}

func preparaScena(t *testing.T) (scena, context.Context) {
	t.Helper()
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	ctx := context.Background()
	q := db.New(p)
	var s scena
	s.pool, s.q = p, q

	crea := func(sigla, nome string) db.Utente {
		var u db.Utente
		err := p.QueryRow(ctx, `INSERT INTO utente (sigla, nome, ufficio) VALUES ($1,$2,'commerciale')
			RETURNING utente_id, sigla, nome, ufficio, ruolo, password_hash, attivo, creato_il`, sigla, nome).
			Scan(&u.UtenteID, &u.Sigla, &u.Nome, &u.Ufficio, &u.Ruolo, &u.PasswordHash, &u.Attivo, &u.CreatoIl)
		if err != nil {
			t.Fatalf("utente %s: %v", sigla, err)
		}
		return u
	}
	s.utenteA, s.utenteB = crea("AA", "Operatore A"), crea("BB", "Operatore B")

	if err := p.QueryRow(ctx, `INSERT INTO cliente (cartella_nas, ragione_sociale) VALUES ('ACME','Acme S.p.A.')
		RETURNING cliente_id`).Scan(&s.cliente.ClienteID); err != nil {
		t.Fatalf("cliente: %v", err)
	}
	s.cliente.CartellaNas = "ACME"

	var convID uuid.UUID
	if err := p.QueryRow(ctx, `INSERT INTO conversazione (canale, chiave_esterna, primo_messaggio_il)
		VALUES ('outlook','CONV-T13', now()) RETURNING conversazione_id`).Scan(&convID); err != nil {
		t.Fatalf("conversazione: %v", err)
	}
	if err := p.QueryRow(ctx, `INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, direzione, data_evento,
		mittente_indirizzo, oggetto) VALUES ('outlook','<t13@acme.example>',$1,'entrata', now(),
		'mario.rossi@acme.example','RFQ contesa') RETURNING messaggio_id`, convID).Scan(&s.messagio); err != nil {
		t.Fatalf("messaggio: %v", err)
	}
	return s, ctx
}

// decideNuovaRFQ ripete, in una transazione propria, ciò che fa l'handler «Nuova RFQ»: blocca il
// messaggio, si ferma se nel frattempo è stato agganciato, altrimenti crea la RFQ e aggancia.
// Restituisce il thread creato, oppure uuid.Nil se ha trovato il messaggio già deciso.
func (s scena) decideNuovaRFQ(ctx context.Context, srv *Server, u db.Utente, oggetto string) (uuid.UUID, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback(ctx)
	q := db.New(tx)
	// si chiama la funzione di produzione, non una sua copia: se domani qualcuno rimettesse
	// GetMessaggio al posto del blocco, questo test fallirebbe invece di continuare a passare
	m, deciso, err := messaggioDaDecidere(ctx, q, s.messagio)
	if err != nil {
		return uuid.Nil, err
	}
	if deciso {
		return uuid.Nil, nil // esito esplicito: la RFQ c'è già, non se ne crea una seconda
	}
	th, err := q.InsertThread(ctx, db.InsertThreadParams{
		ClienteID: s.cliente.ClienteID, Canale: m.Canale, DataInizio: m.DataEvento,
		Oggetto: ptxt(oggetto), CartellaRelativa: ptxt(domain.CartellaThread(s.cliente.CartellaNas, m.DataEvento, "", oggetto)),
		Priorita: 1, CreatoDa: uuid.NullUUID{UUID: u.UtenteID, Valid: true},
	})
	if err != nil {
		return uuid.Nil, err
	}
	if err := srv.agganciaMessaggioAThread(ctx, q, &u, m, th.ThreadID, uuid.NullUUID{}); err != nil {
		return uuid.Nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, err
	}
	return th.ThreadID, nil
}

// T13 — due «Nuova RFQ» concorrenti sullo stesso messaggio.
//
// Prima del blocco di riga entrambe le transazioni leggevano thread_id NULL, creavano una RFQ e
// agganciavano: restavano due RFQ, una delle due senza nessun messaggio e con la sua cartella sul NAS,
// e nessuno dei due operatori vedeva un errore.
func TestDueNuoveRFQConcorrentiNeCreanoUnaSola(t *testing.T) {
	s, ctx := preparaScena(t)
	srv := &Server{Pool: s.pool, Log: testutil.LogSilenzioso()}

	var via sync.WaitGroup
	var fine sync.WaitGroup
	via.Add(1)
	fine.Add(2)
	creati := make([]uuid.UUID, 2)
	errori := make([]error, 2)
	utenti := []db.Utente{s.utenteA, s.utenteB}
	for i := 0; i < 2; i++ {
		go func(i int) {
			defer fine.Done()
			via.Wait()
			creati[i], errori[i] = s.decideNuovaRFQ(ctx, srv, utenti[i], "RFQ contesa")
		}(i)
	}
	via.Done()
	fine.Wait()

	for i, err := range errori {
		if err != nil {
			t.Fatalf("operatore %d: %v", i, err)
		}
	}
	vincitori := 0
	for _, id := range creati {
		if id != uuid.Nil {
			vincitori++
		}
	}
	if vincitori != 1 {
		t.Fatalf("RFQ create = %d, attesa 1: il secondo operatore doveva trovare il messaggio già deciso", vincitori)
	}
	if n := testutil.Conta(t, s.pool, "thread_offerta"); n != 1 {
		t.Errorf("thread in database = %d, atteso 1: una RFQ vuota è rimasta in giro", n)
	}

	// il messaggio è agganciato alla RFQ che ha vinto, e la decisione è nel log
	m, err := s.q.GetMessaggio(ctx, s.messagio)
	if err != nil {
		t.Fatal(err)
	}
	if !m.ThreadID.Valid {
		t.Fatal("il messaggio è rimasto orfano")
	}
	log, err := s.q.ListAgganciaLog(ctx, s.messagio)
	if err != nil {
		t.Fatal(err)
	}
	if len(log) != 1 || log[0].Azione != "aggancia" || !log[0].UtenteID.Valid {
		t.Errorf("log delle decisioni = %+v, attesa una sola riga «aggancia» con l'autore", log)
	}
	if log[0].ThreadID.UUID != m.ThreadID.UUID {
		t.Errorf("il log punta a un thread diverso da quello del messaggio")
	}
}

// T14 — due conferme concorrenti sulla stessa proposta: un solo documento.
func TestDueConfermeConcorrentiDecidonoUnaSolaVolta(t *testing.T) {
	s, ctx := preparaScena(t)

	var allegatoID, propostaID uuid.UUID
	if err := s.pool.QueryRow(ctx, `INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, origine,
		bytes, sha256, path_staging, ricevuto_il)
		VALUES ($1, 1, '6674611A_4.pdf', 'pdf', 'file', 'outlook', 1000, repeat('a',64), 'C:\staging\1.pdf', now())
		RETURNING allegato_id`, s.messagio).Scan(&allegatoID); err != nil {
		t.Fatalf("allegato: %v", err)
	}
	if err := s.pool.QueryRow(ctx, `INSERT INTO documento_proposta (allegato_id, tipo_proposto, confidenza, fonte)
		VALUES ($1, 'disegno_2d', 80, 'nome_file') RETURNING proposta_id`, allegatoID).Scan(&propostaID); err != nil {
		t.Fatalf("proposta: %v", err)
	}

	decide := func(u db.Utente) (bool, error) {
		tx, err := s.pool.Begin(ctx)
		if err != nil {
			return false, err
		}
		defer tx.Rollback(ctx)
		q := db.New(tx)
		p, err := q.BloccaProposta(ctx, propostaID)
		if err != nil {
			return false, err
		}
		if p.Stato != db.StatoPropostaAperta {
			return false, nil // già decisa: chi arriva secondo non decide di nuovo
		}
		// il tempo che serve alla conferma vera (hash, NAS): tiene il blocco abbastanza da rendere
		// certa la sovrapposizione, invece di sperare che le due goroutine si incrocino
		time.Sleep(200 * time.Millisecond)
		if _, err := q.DecidiProposta(ctx, db.DecidiPropostaParams{
			PropostaID: propostaID, Stato: db.StatoPropostaConfermata,
			DecisoDa: uuid.NullUUID{UUID: u.UtenteID, Valid: true},
		}); err != nil {
			return false, err
		}
		if err := tx.Commit(ctx); err != nil {
			return false, err
		}
		return true, nil
	}

	var via, fine sync.WaitGroup
	via.Add(1)
	fine.Add(2)
	deciso := make([]bool, 2)
	errori := make([]error, 2)
	utenti := []db.Utente{s.utenteA, s.utenteB}
	for i := 0; i < 2; i++ {
		go func(i int) {
			defer fine.Done()
			via.Wait()
			deciso[i], errori[i] = decide(utenti[i])
		}(i)
	}
	via.Done()
	fine.Wait()

	for i, err := range errori {
		if err != nil {
			t.Fatalf("conferma %d: %v", i, err)
		}
	}
	if n := 0; true {
		for _, d := range deciso {
			if d {
				n++
			}
		}
		if n != 1 {
			t.Fatalf("conferme andate a segno = %d, attesa 1", n)
		}
	}
	p, err := s.q.GetProposta(ctx, propostaID)
	if err != nil {
		t.Fatal(err)
	}
	if p.Stato != db.StatoPropostaConfermata || !p.DecisoDa.Valid {
		t.Errorf("proposta: stato=%s deciso_da=%v", p.Stato, p.DecisoDa)
	}
}
