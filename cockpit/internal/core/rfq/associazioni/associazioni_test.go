package associazioni

// L1 — il servizio unico delle AR senza database (direttiva 35 §3; piano §3.6, blocco C-1c): i controlli dell'atto e
// della registrazione prima di qualunque lettura, i motivi di chiusura con la loro origine, la traduzione dei rifiuti del
// database. Le prove sul database sono in associazioni_db_test.go (tag integrazione).

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

func codiceDi(t *testing.T, err error) string {
	t.Helper()
	var r *Rifiuto
	if !errors.As(err, &r) {
		t.Fatalf("atteso un Rifiuto, arrivato %v", err)
	}
	return r.Codice
}

// I controlli dell'atto e della registrazione vengono prima di ogni lettura: con q nil una lettura cadrebbe.
func TestV1C1cLAttoSiControllaPrima(t *testing.T) {
	ctx := context.Background()
	ok := Atto{Thread: uuid.New(), Autore: uuid.New(), Chiave: uuid.New(), Conferma: uuid.NullUUID{UUID: uuid.New(), Valid: true}}
	reg := Registrazione{Allegato: uuid.New(), Componente: uuid.New(), ChiaveBozza: "k-s", Provenienza: ProvenienzaManuale}
	casi := []struct {
		nome   string
		a      Atto
		r      Registrazione
		codice string
	}{
		{"senza RFQ", Atto{Autore: ok.Autore, Chiave: ok.Chiave, Conferma: ok.Conferma}, reg, CodiceAttoNonValido},
		{"senza autore", Atto{Thread: ok.Thread, Chiave: ok.Chiave, Conferma: ok.Conferma}, reg, CodiceAttoNonValido},
		{"senza chiave", Atto{Thread: ok.Thread, Autore: ok.Autore, Conferma: ok.Conferma}, reg, CodiceAttoNonValido},
		{"nota troppo lunga", Atto{Thread: ok.Thread, Autore: ok.Autore, Chiave: ok.Chiave, Conferma: ok.Conferma, Nota: strings.Repeat("x", 1001)}, reg, CodiceAttoNonValido},
		{"senza allegato", ok, Registrazione{Componente: reg.Componente, ChiaveBozza: "k"}, CodiceAttoNonValido},
		{"senza componente", ok, Registrazione{Allegato: reg.Allegato, ChiaveBozza: "k"}, CodiceAttoNonValido},
		{"senza nodo della bozza", ok, Registrazione{Allegato: reg.Allegato, Componente: reg.Componente, Provenienza: ProvenienzaManuale}, CodiceAttoNonValido},
		// direttiva 39, D-PB03 A [U]: la PB applicata porta la sua provenienza; il registro d'origine solo con motore o corretta
		{"senza provenienza", ok, Registrazione{Allegato: reg.Allegato, Componente: reg.Componente, ChiaveBozza: "k"}, CodiceProvenienzaNonValida},
		{"provenienza fuori elenco", ok, Registrazione{Allegato: reg.Allegato, Componente: reg.Componente, ChiaveBozza: "k", Provenienza: "browser"},
			CodiceProvenienzaNonValida},
		{"registro con una PB manuale", ok, Registrazione{Allegato: reg.Allegato, Componente: reg.Componente, ChiaveBozza: "k", Provenienza: ProvenienzaManuale,
			Importazione: uuid.NullUUID{UUID: uuid.New(), Valid: true}}, CodiceProvenienzaNonValida},
		// D-AR01 A [U]: la AR di R5 nasce solo con la conferma; un gesto legacy passa da RegistraLegacy
		{"senza conferma", Atto{Thread: ok.Thread, Autore: ok.Autore, Chiave: ok.Chiave}, reg, CodiceConfermaObbligatoria},
	}
	for _, c := range casi {
		for nome, f := range map[string]func(context.Context, Atto, Registrazione) (Esito, error){
			"Registra":    func(ctx context.Context, a Atto, r Registrazione) (Esito, error) { return Registra(ctx, nil, a, r) },
			"Riassocia":   func(ctx context.Context, a Atto, r Registrazione) (Esito, error) { return Riassocia(ctx, nil, a, r) },
			"Sostituisci": func(ctx context.Context, a Atto, r Registrazione) (Esito, error) { return Sostituisci(ctx, nil, a, r) },
		} {
			if _, err := f(ctx, c.a, c.r); codiceDi(t, err) != c.codice {
				t.Errorf("%s, %s: %v", nome, c.nome, err)
			}
		}
	}
	// D-AR03 D [U]: il gesto legacy non ha conferma, e vuole allegato e componente (prima di ogni lettura)
	senza := Atto{Thread: ok.Thread, Autore: ok.Autore, Chiave: ok.Chiave}
	for nome, f := range map[string]func(context.Context, Atto, uuid.UUID, uuid.UUID) (Esito, error){
		"RegistraLegacy": func(ctx context.Context, a Atto, al, co uuid.UUID) (Esito, error) {
			return RegistraLegacy(ctx, nil, a, al, co)
		},
		"SostituisciLegacy": func(ctx context.Context, a Atto, al, co uuid.UUID) (Esito, error) {
			return SostituisciLegacy(ctx, nil, a, al, co)
		},
	} {
		if _, err := f(ctx, ok, reg.Allegato, reg.Componente); codiceDi(t, err) != CodiceAttoNonValido {
			t.Errorf("%s con la conferma: %v", nome, err)
		}
		if _, err := f(ctx, senza, uuid.Nil, reg.Componente); codiceDi(t, err) != CodiceAttoNonValido {
			t.Errorf("%s senza allegato: %v", nome, err)
		}
		if _, err := f(ctx, senza, reg.Allegato, uuid.Nil); codiceDi(t, err) != CodiceAttoNonValido {
			t.Errorf("%s senza componente: %v", nome, err)
		}
	}
}

// I motivi di chiusura: svincolo_conferma e' di R5 (con la conferma e il componente), gli altri senza conferma;
// sostituita la scrive solo Sostituisci. L'origine dell'evento segue il motivo.
func TestV1C1cIMotiviDiChiusura(t *testing.T) {
	ctx := context.Background()
	conf := uuid.NullUUID{UUID: uuid.New(), Valid: true}
	a := Atto{Thread: uuid.New(), Autore: uuid.New(), Chiave: uuid.New()}
	ar := a
	ar.Conferma = conf
	al, comp := uuid.New(), uuid.New()
	casi := []struct {
		nome   string
		a      Atto
		s      Scollegamento
		codice string
	}{
		{"sostituita non e' di Scollega", a, Scollegamento{Allegato: al, Motivo: MotivoSostituita}, CodiceMotivoNonAmmesso},
		{"motivo sconosciuto", a, Scollegamento{Allegato: al, Motivo: "boh"}, CodiceMotivoNonAmmesso},
		{"sgancio_legacy con la conferma", ar, Scollegamento{Allegato: al, Motivo: MotivoSgancioLegacy}, CodiceMotivoNonAmmesso},
		{"svincolo senza conferma", a, Scollegamento{Allegato: al, Componente: comp, Motivo: MotivoSvincoloConferma}, CodiceConfermaObbligatoria},
		{"svincolo senza componente", ar, Scollegamento{Allegato: al, Motivo: MotivoSvincoloConferma}, CodiceAttoNonValido},
		{"scollega_file con la conferma", ar, Scollegamento{Allegato: al, Motivo: MotivoScollegaFile}, CodiceMotivoNonAmmesso},
		{"senza allegato", a, Scollegamento{Motivo: MotivoScollegaFile}, CodiceAttoNonValido},
	}
	for _, c := range casi {
		if _, err := Scollega(ctx, nil, c.a, c.s); codiceDi(t, err) != c.codice {
			t.Errorf("%s: %v", c.nome, err)
		}
	}
	for m, o := range map[string]string{MotivoSvincoloConferma: OrigineEventoConfermaBOM, MotivoScollegaFile: OrigineEventoPonte,
		MotivoFileGenerale: OrigineEventoLegacy, MotivoScartoLegacy: OrigineEventoLegacy, MotivoSgancioLegacy: OrigineEventoLegacy,
		MotivoSostituita: ""} {
		if got := OrigineDelMotivo(m); got != o {
			t.Errorf("origine di %s: %q, attesa %q", m, got, o)
		}
	}
	if _, err := LegaSorgente(ctx, nil, ar, LegameSorgente{Allegato: al, Prodotto: comp}); codiceDi(t, err) != CodiceAttoNonValido {
		t.Errorf("il legame della sorgente con una conferma: %v", err)
	}
}

// I rifiuti del database diventano Rifiuto: la corsa sull'indice della AR attiva e' gia_associata_altrove, la BOM
// congelata bom_congelata, gli altri vincoli vincolo_associazione; il resto passa com'e'.
func TestV1C1cLaTraduzioneDeiRifiuti(t *testing.T) {
	casi := []struct {
		err    error
		codice string
	}{
		{&pgconn.PgError{Code: "23505", ConstraintName: "ux_fra_attiva_allegato"}, CodiceGiaAssociataAltrove},
		{&pgconn.PgError{Code: "23505", ConstraintName: "ux_fra_attiva_coppia"}, CodiceGiaAssociataAltrove},
		{&pgconn.PgError{Code: "23505", ConstraintName: "ux_fra_chiave"}, CodiceVincolo},
		{&pgconn.PgError{Code: "P4X10", Message: "file_rfq_associazione: ..."}, CodiceVincolo},
		{&pgconn.PgError{Code: "23503", ConstraintName: "fk_fra_conferma_componente"}, CodiceVincolo},
		{&pgconn.PgError{Code: "BOM01"}, CodiceBomCongelata},
	}
	for _, c := range casi {
		if got := codiceDi(t, traduci(c.err)); got != c.codice {
			t.Errorf("%v: %s, atteso %s", c.err, got, c.codice)
		}
	}
	altro := errors.New("rete")
	if traduci(altro) != altro {
		t.Error("un errore che non e' del database cambia")
	}
	if e := traduci(&pgconn.PgError{Code: "40P01"}); errors.As(e, new(*Rifiuto)) {
		t.Error("uno stallo diventa un rifiuto di dominio")
	}
}
