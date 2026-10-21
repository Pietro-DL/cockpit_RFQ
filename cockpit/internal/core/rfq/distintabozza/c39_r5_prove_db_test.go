//go:build integrazione

// L4 — R5 formato 3 completo, le prove che mancavano (direttiva 39, blocco «R5 completo» della corsia persistenza; direttiva
// 35 prove 8 e 10; EM §9.5 passi 1-4, 11), sul database di prova:
//   - Prova 8: il registrato escluso che un altro prodotto tiene resta nella working (escluso condiviso), esce dal
//     perimetro di questo prodotto, con la condivisione dichiarata in R4;
//   - due R5 in concorrenza sulla stessa bozza: con due chiavi una sola conferma (l'altra 409 BOZZA_VERSIONE), con la
//     stessa chiave il secondo e' riproposto; mai due conferme o due AR;
//   - la connessione S che non arriva (il pool esaurito): 503 CALCOLO_OCCUPATO non registrato, niente scritto;
//   - Prova 10: la bozza uguale alla conferma di base: 422 NESSUNA_MODIFICA, niente scritto.
// Dati sintetici (ACME, codici ACM-xxxx).

package distintabozza

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/testutil"
)

// Prova 8 (dir. 35) nella parte di R5: il pezzo della prima conferma e' anche nella working di un altro prodotto; la bozza
// lo esclude: R4 dichiara la condivisione, R5 lo lascia nella working (escluso condiviso, non archiviato), toglie solo
// l'arco di questo prodotto e lascia la AR (D-AR09 «Mantieni»).
func TestV1D39R5Prova8CondivisoDB(t *testing.T) {
	p, s := poolPulito(t)
	_, pezzo, _ := primaConfermaDB(t, p, s)
	esegui3(t, p, `INSERT INTO componente_relazione (thread_id, padre_id, figlio_id, qta, origine, confermato_da) VALUES ($1, $2, $3, 1, 'manuale', $4)`,
		s.thread, s.prodotto, pezzo, s.utente)
	b, err := creaDB(t, p, s, s.altroFinito, uuid.New(), map[string]any{"partenza": PartenzaConferma})
	if err != nil {
		t.Fatalf("da-conferma-1: %v", err)
	}
	m, _ := snapshotDellaVersione(t, p, b.BozzaID, 1)
	kp := ""
	var primaNodo map[string]any
	for _, x := range m["nodi"].([]any) {
		n := x.(map[string]any)
		if n["componente_id"] == pezzo.String() {
			kp, primaNodo = n["chiave_bozza"].(string), map[string]any{"op": n["op"]}
			n["op"] = OpEsclude
		}
	}
	if kp == "" {
		t.Fatal("il pezzo non c'e' nella bozza")
	}
	ex := map[string]any{"id": "ex-p", "tipo": "escludi", "componente_id": pezzo, "motivo": "non fa parte di questo prodotto"}
	m["decisioni"] = append(m["decisioni"].([]any), ex)
	sc := scena3DB{scenaDB: s, p: p}
	if _, err := sc.salva3Su(t, b.BozzaID, s.altroFinito, 1, uuid.New(), m, []Azione{azione3(t, AzDecisioneAggiunta, "", nil, ex),
		azione3(t, AzNodoModificato, kp, primaNodo, map[string]any{"op": OpEsclude})}); err != nil {
		t.Fatalf("R2 dell'esclusione: %v", err)
	}
	rp := mustRiepilogoDB(t, p, s.thread, corpoDellaVersione(t, p, b.BozzaID, 2), s.utente)
	if len(rp.Blocchi) != 0 {
		t.Fatalf("R4: %v %+v", codici(rp.Blocchi), rp.Blocchi)
	}
	condiviso := false
	for _, c := range rp.Condivisione {
		if c.ProdottoID != s.prodotto {
			continue
		}
		for _, e := range c.Elementi {
			condiviso = condiviso || (e.ComponenteID != nil && *e.ComponenteID == pezzo)
		}
	}
	if !condiviso || len(trova(rp.Avvisi, AvvisoEsclusoConDocumenti)) != 1 {
		t.Fatalf("la condivisione dichiarata in R4: %+v avvisi %v", rp.Condivisione, codici(rp.Avvisi))
	}
	out, err := confermaR5DB(t, p, s, richiestaR5(t, p, b.BozzaID, 2, rp.FirmaAnteprima, uuid.New(), nil), OpzioniR5{})
	if err != nil {
		t.Fatalf("R5: %v", err)
	}
	if len(out.EsclusiCondivisi) != 1 || out.EsclusiCondivisi[0] != pezzo || len(out.IdentitaArchiviate) != 0 || len(out.ArchiTolti) != 1 {
		t.Fatalf("risposta: %+v", out)
	}
	if n := conta3(t, p, `SELECT count(*) FROM componente WHERE componente_id = $1 AND archiviato_il IS NULL`, pezzo); n != 1 {
		t.Fatal("l'escluso condiviso resta nella working")
	}
	if n := conta3(t, p, `SELECT count(*) FROM componente_relazione WHERE figlio_id = $1 AND padre_id = $2`, pezzo, s.altroFinito); n != 0 {
		t.Fatal("l'arco di questo prodotto e' tolto")
	}
	if n := conta3(t, p, `SELECT count(*) FROM componente_relazione WHERE figlio_id = $1 AND padre_id = $2`, pezzo, s.prodotto); n != 1 {
		t.Fatal("l'arco dell'altro prodotto resta")
	}
	if n := conta3(t, p, `SELECT count(*) FROM conferma_strutturale_componente WHERE conferma_id = $1 AND componente_id = $2`, out.ConfermaID, pezzo); n != 0 {
		t.Fatal("l'escluso esce dal perimetro certificato")
	}
	if n := conta3(t, p, `SELECT count(*) FROM file_rfq_associazione WHERE componente_id = $1 AND stato = 'attiva'`, pezzo); n != 1 {
		t.Fatal("la AR dell'escluso resta (Mantieni)")
	}
}

// attendiInAttesa: aspetta (al piu' 10 s) che n sessioni del database di prova aspettino un lucchetto.
func attendiInAttesa(t *testing.T, p *pgxpool.Pool, n int) {
	t.Helper()
	fine := time.Now().Add(10 * time.Second)
	for time.Now().Before(fine) {
		var k int
		if err := p.QueryRow(context.Background(), `SELECT count(*) FROM pg_stat_activity WHERE datname = current_database()
			AND wait_event_type = 'Lock' AND pid <> pg_backend_pid()`).Scan(&k); err != nil {
			t.Fatal(err)
		}
		if k >= n {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("le %d conferme non sono arrivate al lucchetto della RFQ", n)
}

// Due R5 in concorrenza sulla stessa bozza (EM §9.5; direttiva 39 §7: nessuna doppia conferma o doppia AR sui retry): una
// terza transazione tiene la RFQ, le due conferme partono insieme e se la contendono.
func TestV1D39R5ConcorrenzaFraDueR5DB(t *testing.T) {
	for _, stessaChiave := range []bool{false, true} {
		nome := "chiavi diverse"
		if stessaChiave {
			nome = "stessa chiave"
		}
		t.Run(nome, func(t *testing.T) {
			p, s := poolPulito(t)
			ctx := context.Background()
			pdf := allegatoFileDB(t, p, s.thread, "ACM-6500.pdf", "ACM-6500", '5')
			bozza := bozzaManualeDB(t, p, s, s.altroFinito, "ACM-6500", pdf)
			s8DB(t, p, s, s.altroFinito, s.allegato2, shaProva('c'), "ACM-6000", true)
			v := soloProdottoSuDB(t, p, s, s.altroFinito, bozza, 2, s.allegato2)
			rp := mustRiepilogoDB(t, p, s.thread, corpoDellaVersione(t, p, bozza, v), s.utente)
			k1, k2 := uuid.New(), uuid.New()
			if stessaChiave {
				k2 = k1
			}
			r1, r2 := richiestaR5(t, p, bozza, v, rp.FirmaAnteprima, k1, nil), richiestaR5(t, p, bozza, v, rp.FirmaAnteprima, k2, nil)
			tx, err := p.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.New(tx).BloccaThread(ctx, s.thread); err != nil {
				t.Fatal(err)
			}
			var wg sync.WaitGroup
			esiti := make([]ConfermaR5, 2)
			errori := make([]error, 2)
			for i, r := range []RichiestaR5{r1, r2} {
				wg.Add(1)
				go func(i int, r RichiestaR5) {
					defer wg.Done()
					esiti[i], errori[i] = Conferma3(ctx, p, s.thread, r, OpzioniR5{UtenteID: s.utente, Motore: motoreFintoDB})
				}(i, r)
			}
			attendiInAttesa(t, p, 2)
			if err := tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			wg.Wait()
			riuscite, riproposte := 0, 0
			for i := range esiti {
				switch {
				case errori[i] == nil && !esiti[i].Riproposto:
					riuscite++
				case errori[i] == nil && esiti[i].Riproposto:
					riproposte++
				default:
					var e *Errore
					if stessaChiave || !errors.As(errori[i], &e) || e.Codice != CodiceBozzaVersione || e.Motivo != MotivoBozzaChiusa {
						t.Fatalf("la seconda conferma: %v", errori[i])
					}
				}
			}
			if riuscite != 1 || (stessaChiave && riproposte != 1) || (!stessaChiave && riproposte != 0) {
				t.Fatalf("esiti: %+v errori %v", esiti, errori)
			}
			if stessaChiave && esiti[0].ConfermaID != esiti[1].ConfermaID {
				t.Fatal("la stessa chiave riproposta con la stessa conferma")
			}
			if n := conta3(t, p, `SELECT count(*) FROM conferma_strutturale WHERE prodotto_id = $1`, s.altroFinito); n != 1 {
				t.Fatalf("conferme: %d", n)
			}
			if n := conta3(t, p, `SELECT count(*) FROM file_rfq_associazione WHERE allegato_id = $1`, pdf); n != 1 {
				t.Fatalf("AR: %d", n)
			}
			if n := conta3(t, p, `SELECT count(*) FROM associazione_evento WHERE allegato_id = $1 AND azione = 'preassociazione'`, pdf); n != 1 {
				t.Fatalf("eventi della AR: %d", n)
			}
		})
	}
}

// EM §9.5 passo 4: la connessione S non arriva entro l'attesa (qui un pool di una sola connessione, che tiene T): 503
// CALCOLO_OCCUPATO non registrato, niente scritto; con la connessione libera la stessa richiesta passa.
func TestV1D39R5SenzaConnessioneS503DB(t *testing.T) {
	p, s := poolPulito(t)
	ctx := context.Background()
	c, err := creaDB(t, p, s, s.altroFinito, uuid.New(), map[string]any{"partenza": PartenzaVuota})
	if err != nil {
		t.Fatal(err)
	}
	s8DB(t, p, s, s.altroFinito, s.allegato2, shaProva('c'), "ACM-6000", true)
	v := soloProdottoSuDB(t, p, s, s.altroFinito, c.BozzaID, 1, s.allegato2)
	rp := mustRiepilogoDB(t, p, s.thread, corpoDellaVersione(t, p, c.BozzaID, v), s.utente)
	cfg, err := pgxpool.ParseConfig(testutil.DSN(t))
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = 1
	uno, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer uno.Close()
	req := richiestaR5(t, p, c.BozzaID, v, rp.FirmaAnteprima, uuid.New(), nil)
	prima := fotoR5(t, p)
	_, err = Conferma3(ctx, uno, s.thread, req, OpzioniR5{UtenteID: s.utente, Motore: motoreFintoDB, AttesaS: 300 * time.Millisecond})
	r := rifiutoR5Di(t, err, CodiceCalcoloOccupato)
	if !r.NonRegistrato {
		t.Fatalf("503 non registrato: %+v", r)
	}
	if d := diffFoto(prima, fotoR5(t, p)); len(d) > 0 {
		t.Fatalf("il 503 ha scritto in %v", d)
	}
	out, err := confermaR5DB(t, p, s, req, OpzioniR5{})
	if err != nil || out.Riproposto || out.NumeroConferma != 1 {
		t.Fatalf("con la stessa chiave, dopo: %+v %v", out, err)
	}
}

// Prova 10 (dir. 35) nella parte di R5: la bozza da-conferma-1 senza modifiche e' uguale alla conferma di base: 422
// NESSUNA_MODIFICA, niente scritto.
func TestV1D39R5NessunaModificaDB(t *testing.T) {
	p, s := poolPulito(t)
	primaConfermaDB(t, p, s)
	b, err := creaDB(t, p, s, s.altroFinito, uuid.New(), map[string]any{"partenza": PartenzaConferma})
	if err != nil {
		t.Fatalf("da-conferma-1: %v", err)
	}
	rp := mustRiepilogoDB(t, p, s.thread, corpoDellaVersione(t, p, b.BozzaID, 1), s.utente)
	if !rp.UgualeAllaBase || len(rp.Blocchi) != 0 {
		t.Fatalf("R4: uguale %v, blocchi %v %+v", rp.UgualeAllaBase, codici(rp.Blocchi), rp.DifferenzeDallaBase)
	}
	prima := fotoR5(t, p)
	_, err = confermaR5DB(t, p, s, richiestaR5(t, p, b.BozzaID, 1, rp.FirmaAnteprima, uuid.New(), nil), OpzioniR5{})
	var e *Errore
	if !errors.As(err, &e) || e.Codice != CodiceNessunaModifica {
		t.Fatalf("NESSUNA_MODIFICA: %v", err)
	}
	if d := diffFoto(prima, fotoR5(t, p)); len(d) > 0 {
		t.Fatalf("NESSUNA_MODIFICA ha scritto in %v", d)
	}
}
