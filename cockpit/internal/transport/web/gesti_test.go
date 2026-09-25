package web

// L1 — i pezzi dei gesti sul Fascicolo che non hanno bisogno di un database: la quantita' di un arco,
// letta a 32 bit; la frase di un rifiuto, che sia detto qui o dal core; e un deadlock (40P01), che per
// l'operatore e' «riprova», non un SQLSTATE.

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"promatec/cockpit/internal/core/rfq/fascicolo"
)

// La quantita' di un arco si legge a 32 bit: letta come int e poi convertita, 4294967297 diventava 1
// senza che nessuno lo dicesse. Vuota vale 1; tutto cio' che non e' un numero a 32 bit e' un rifiuto.
func TestLaQuantitaDiUnArcoSiLeggeATrentaDueBit(t *testing.T) {
	for _, c := range []struct {
		v      string
		qta    int32
		valida bool
	}{
		{"", 1, true},
		{" 3 ", 3, true},
		{"2147483647", 2147483647, true},
		{"4294967297", 0, false},
		{"2147483648", 0, false},
		{"tre", 0, false},
		{"1.5", 0, false},
	} {
		qta, err := qtaDal(c.v)
		if (err == nil) != c.valida {
			t.Errorf("qtaDal(%q): errore %v, atteso valida=%v", c.v, err, c.valida)
			continue
		}
		if err != nil {
			var r rifiuto
			if !errors.As(err, &r) || !strings.Contains(spiegaErrore(err), "quantità non valida") {
				t.Errorf("qtaDal(%q): il rifiuto non arriva all'operatore: %v", c.v, err)
			}
			continue
		}
		if qta != c.qta {
			t.Errorf("qtaDal(%q) = %d, atteso %d", c.v, qta, c.qta)
		}
	}
}

// Un «no» detto qui (rifiuto) e uno detto dal core (fascicolo.Rifiuto) sono lo stesso tipo: la frase
// arriva all'operatore uguale, anche avvolta in un altro errore.
func TestUnRifiutoDelCoreEQuelloDelWebSonoLoStesso(t *testing.T) {
	for _, err := range []error{
		rifiuto("la BOM è congelata"),
		fascicolo.Rifiuto("la BOM è congelata"),
		fmt.Errorf("assegna: %w", fascicolo.Rifiuto("la BOM è congelata")),
	} {
		if got := spiegaErrore(err); got != "la BOM è congelata" {
			t.Errorf("spiegaErrore(%v) = %q", err, got)
		}
	}
}

// Due gesti sulla stessa RFQ che si aspettano a vicenda: PostgreSQL ne ferma uno (40P01). Per chi ha
// premuto non e' un errore del server ne' un SQLSTATE: niente e' cambiato, e si riprova.
func TestUnDeadlockDiceDiRiprovare(t *testing.T) {
	err := fmt.Errorf("documento: %w", &pgconn.PgError{Code: "40P01", Message: "deadlock detected"})
	got := spiegaErrore(err)
	if !strings.Contains(got, "riprova") || strings.Contains(got, "40P01") || strings.Contains(got, "deadlock") {
		t.Errorf("40P01: %q", got)
	}
}
