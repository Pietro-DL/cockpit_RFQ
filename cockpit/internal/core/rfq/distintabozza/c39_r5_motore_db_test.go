//go:build integrazione

// L4 — R5 formato 3 con il Motore A nella connessione S (riepilogo3_motore.go, conferma3.go; EM §9.5 con U2 §6.3.5), corsia
// persistenza: il motore occupato o non pronto da' 503 CALCOLO_OCCUPATO (ma una bozza senza sorgente resta 422 BLOCCHI),
// scaduto 504 CALCOLO_SCADUTO, un altro errore o nessun motore 500 ERRORE_CALCOLO, tutti non registrati e senza scritture;
// il riepilogo senza il motore (leggero) ha un'altra firma: R5 risponde 409 con il riepilogo con il motore; i conflitti
// del motore entrano nella firma e la conferma passa (la verifica non e' un gate). Il motore e' finto (il vero lo da'
// l'adattatore web: le sue prove L4 sono in transport/web). Dati sintetici.

package distintabozza

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/core/valutazione"
	"promatec/cockpit/internal/platform/db"
)

func TestV1D39R5ConIlMotoreDB(t *testing.T) {
	p, s := poolPulito(t)
	// la bozza senza STEP con il motore occupato: 422 BLOCCHI con il gate, non 503
	a := allegatoFileDB(t, p, s.thread, "ACM-6500.pdf", "ACM-6500", '5')
	manuale := bozzaManualeDB(t, p, s, s.prodotto, "ACM-6500", a)
	rpM := mustRiepilogoDB(t, p, s.thread, corpoDellaVersione(t, p, manuale, 2), s.utente)
	occupato := func(context.Context, pgx.Tx, uuid.UUID, uuid.UUID) (*IngressoMotoreR4, error) {
		return nil, errors.Join(errors.New("tutti i posti presi"), ErrMotoreOccupato)
	}
	prima := fotoR5(t, p)
	_, err := confermaR5DB(t, p, s, richiestaR5(t, p, manuale, 2, rpM.FirmaAnteprima, uuid.New(), nil), OpzioniR5{Motore: occupato})
	if r := rifiutoR5Di(t, err, CodiceBlocchi); len(trova(r.Blocchi, BloccoStepSorgenteNonConfermata)) != 1 {
		t.Fatalf("senza sorgente e con il motore occupato: %+v", r.Blocchi)
	}
	// la bozza con la sorgente S8 (sola radice): pronta per R5
	c, err := creaDB(t, p, s, s.altroFinito, uuid.New(), map[string]any{"partenza": PartenzaVuota})
	if err != nil {
		t.Fatal(err)
	}
	s8DB(t, p, s, s.altroFinito, s.allegato2, shaProva('c'), "ACM-6000", true)
	v := soloProdottoSuDB(t, p, s, s.altroFinito, c.BozzaID, 1, s.allegato2)
	rp := mustRiepilogoDB(t, p, s.thread, corpoDellaVersione(t, p, c.BozzaID, v), s.utente)
	if len(rp.Blocchi) != 0 || !rp.Confermabile || !rp.AvvisiMotoreCalcolati {
		t.Fatalf("R4 con il motore: %v %v %v", codici(rp.Blocchi), rp.Confermabile, rp.AvvisiMotoreCalcolati)
	}
	prima = fotoR5(t, p)
	for _, x := range []struct {
		nome   string
		motore CalcoloMotoreR5
		codice string
	}{
		{"occupato", occupato, CodiceCalcoloOccupato},
		{"non pronto", func(context.Context, pgx.Tx, uuid.UUID, uuid.UUID) (*IngressoMotoreR4, error) {
			return nil, ErrMotoreNonPronto
		}, CodiceCalcoloOccupato},
		{"scaduto", func(context.Context, pgx.Tx, uuid.UUID, uuid.UUID) (*IngressoMotoreR4, error) {
			return nil, ErrMotoreScaduto
		}, CodiceCalcoloScaduto},
		{"guasto", func(context.Context, pgx.Tx, uuid.UUID, uuid.UUID) (*IngressoMotoreR4, error) {
			return nil, errors.New("panic")
		}, CodiceErroreCalcolo},
	} {
		_, err := confermaR5DB(t, p, s, richiestaR5(t, p, c.BozzaID, v, rp.FirmaAnteprima, uuid.New(), nil), OpzioniR5{Motore: x.motore})
		if r := rifiutoR5Di(t, err, x.codice); !r.NonRegistrato {
			t.Fatalf("%s: non registrato: %+v", x.nome, r)
		}
	}
	// nessun motore: 500 ERRORE_CALCOLO (direttamente, senza l'aiuto che mette il motore finto)
	_, err = Conferma3(context.Background(), p, s.thread, richiestaR5(t, p, c.BozzaID, v, rp.FirmaAnteprima, uuid.New(), nil), OpzioniR5{UtenteID: s.utente})
	rifiutoR5Di(t, err, CodiceErroreCalcolo)
	if d := diffFoto(prima, fotoR5(t, p)); len(d) > 0 {
		t.Fatalf("R5 con il motore che non risponde ha scritto in %v", d)
	}
	// il riepilogo leggero ha un'altra firma: 409 con il riepilogo con il motore
	rl, err := riepilogoLeggeroDB(t, p, s.thread, corpoDellaVersione(t, p, c.BozzaID, v), s.utente)
	if err != nil {
		t.Fatal(err)
	}
	if rl.Confermabile || rl.FirmaAnteprima == rp.FirmaAnteprima {
		t.Fatalf("leggero: %v %s", rl.Confermabile, rl.FirmaAnteprima)
	}
	_, err = confermaR5DB(t, p, s, richiestaR5(t, p, c.BozzaID, v, rl.FirmaAnteprima, uuid.New(), nil), OpzioniR5{})
	if r := rifiutoR5Di(t, err, CodiceAnteprimaSuperata); r.Riepilogo == nil || r.Riepilogo.FirmaAnteprima != rp.FirmaAnteprima {
		t.Fatalf("409 con il riepilogo con il motore: %+v", r.Riepilogo)
	}
	// i conflitti del motore sul prodotto: nella firma (la firma di R4 senza conflitti non vale), la conferma passa con la
	// firma giusta
	conConflitti := func(context.Context, pgx.Tx, uuid.UUID, uuid.UUID) (*IngressoMotoreR4, error) {
		pv := &valutazione.ProdottoValutato{}
		pv.BOM.Nomenclatura.Conflitti = []string{"nodo:x"}
		return &IngressoMotoreR4{Prodotto: pv}, nil
	}
	_, err = confermaR5DB(t, p, s, richiestaR5(t, p, c.BozzaID, v, rp.FirmaAnteprima, uuid.New(), nil), OpzioniR5{Motore: conConflitti})
	r := rifiutoR5Di(t, err, CodiceAnteprimaSuperata)
	if r.Riepilogo == nil || len(r.Riepilogo.ConflittiMotore) != 1 || r.Riepilogo.VerificaPrevista.ConflittiMotoreCalcolati != true {
		t.Fatalf("conflitti nel riepilogo nuovo: %+v", r.Riepilogo)
	}
	out, err := confermaR5DB(t, p, s, richiestaR5(t, p, c.BozzaID, v, r.Riepilogo.FirmaAnteprima, uuid.New(), nil), OpzioniR5{Motore: conConflitti})
	if err != nil || out.NumeroConferma != 1 {
		t.Fatalf("R5 con i conflitti del motore (la verifica non e' un gate): %v %+v", err, out)
	}
}

// riepilogoLeggeroDB: R4 senza ?avvisi_motore=1.
func riepilogoLeggeroDB(t *testing.T, p interface {
	BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error)
}, thread uuid.UUID, corpo map[string]any, utente uuid.UUID) (RiepilogoR4, error) {
	t.Helper()
	r, err := LeggiRichiestaR4(mustJSON(corpo))
	if err != nil {
		return RiepilogoR4{}, err
	}
	ctx := context.Background()
	tx, err := p.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	return Riepilogo3(ctx, db.New(tx), thread, r, OpzioniR4{UtenteID: utente})
}
