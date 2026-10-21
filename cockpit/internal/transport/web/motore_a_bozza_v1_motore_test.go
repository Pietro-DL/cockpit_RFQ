package web

// L1 — il calcolo del motore di R4 e R5 (motore_a_bozza_v1_motore.go): gli errori del servizio nei codici del core, il
// prodotto valutato e i gruppi degli altri prodotti, l'Iniziatore dentro S solo in lettura. Dati sintetici.

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/rfq/distintabozza"
	"promatec/cockpit/internal/core/valutazione"
	motorea_web "promatec/cockpit/internal/transport/web/motore_a"
)

func TestV1MotoreR4R5ErroriEIngresso(t *testing.T) {
	for _, c := range []struct {
		err    error
		atteso error
	}{{motorea_web.ErrOccupato, distintabozza.ErrMotoreOccupato}, {motorea_web.ErrRegole, distintabozza.ErrMotoreNonPronto},
		{fmt.Errorf("x: %w", motorea_web.ErrTempo), distintabozza.ErrMotoreScaduto}, {context.DeadlineExceeded, distintabozza.ErrMotoreScaduto}} {
		if e := erroreMotoreV1(c.err); !errors.Is(e, c.atteso) {
			t.Errorf("%v: %v", c.err, e)
		}
	}
	altro := errors.New("panic")
	if e := erroreMotoreV1(altro); e != altro {
		t.Errorf("un altro errore resta com'e': %v", e)
	}
	p, q := uuid.New(), uuid.New()
	a := uuid.New()
	ris := &motorea_web.Risultato{EsitoThread: &valutazione.EsitoThread{
		ProdottiValutati: []valutazione.ProdottoValutato{{Rif: "componente:" + p.String(), ComponenteID: &p,
			StruttureCandidate: []valutazione.StrutturaCandidata{{AllegatoID: a, Sha256: "s1", Radice: "nodo:s1:#1"}}},
			{Rif: "componente:" + q.String(), ComponenteID: &q, StruttureCandidate: []valutazione.StrutturaCandidata{{AllegatoID: a, Sha256: "s2", Radice: "nodo:s2:#1"}}},
			{Rif: "identificativo:x"}},
		Ancoraggi: ancoraggio.EsitoAncoraggi{}}}
	in := ingressoMotoreV1(ris, p)
	if in.Prodotto == nil || *in.Prodotto.ComponenteID != p || len(in.Gruppi) != 1 || in.Gruppi[0].Sha256 != "s1" || len(in.AltriGruppi) != 1 ||
		len(in.AltriGruppi[q]) != 1 || in.AltriGruppi[q][0].Sha256 != "s2" || in.Importa == nil {
		t.Fatalf("ingresso del motore: %+v", in)
	}
	if in := ingressoMotoreV1(ris, uuid.New()); in.Prodotto != nil || len(in.Gruppi) != 0 || len(in.AltriGruppi) != 2 {
		t.Fatalf("prodotto non valutato: %+v", in)
	}
	if _, err := (iniziatoreInS{}).BeginTx(context.Background(), pgx.TxOptions{AccessMode: pgx.ReadWrite}); err == nil {
		t.Fatal("l'Iniziatore dentro S accetta solo la sola lettura")
	}
}
