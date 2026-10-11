package motore_a

// L1 — giro 5, A1d-Q2: lo snapshot unico (snapshot.go; bozza delle risposte §14 P2/Q2; F-04), senza database, con una
// transazione finta:
//   - nelloSnapshot apre UNA transazione REPEATABLE READ READ ONLY e nient'altro, la chiude sempre con il rollback e mai
//     con il commit, anche dopo un errore; senza Pool non apre niente (ErrLettura);
//   - il savepoint che il caricatore riceve: BeginTx con le opzioni dello snapshot apre un savepoint della transazione
//     esterna (Tx.Begin), con altre opzioni rifiuta senza aprire niente;
//   - leggiSnapshot comincia con GetThread: una RFQ che non c'è è ErrRFQInesistente senza nessun'altra lettura (né la
//     fase, né il savepoint del caricatore, né le dichiarazioni); un errore di GetThread è ErrLettura;
//   - Esiste: solo GetThread, nella stessa forma di transazione.
// Che i due SHOW del caricatore leggano «on» e «repeatable read» dentro il savepoint, e che la transazione sia davvero in
// sola lettura, lo provano le L4 (servizio_db_test.go) su PostgreSQL.
//
// I clienti di questi test sono inventati: UUID 00000000-0000-4000-8000-0000000000nn.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// maSnapTx: una transazione finta con Begin (il savepoint). Le letture prendono le risposte di maStepTx; Begin registra
// la chiamata e dà un'altra transazione finta.
type maSnapTx struct {
	maStepTx
	savepoint int
}

func (x *maSnapTx) Begin(context.Context) (pgx.Tx, error) {
	x.savepoint++
	return &maStepTx{}, nil
}

// QueryRow, Query, Exec, Rollback e Commit di maStepTx, sul puntatore giusto (maSnapTx incorpora il valore).
func (x *maSnapTx) QueryRow(ctx context.Context, sql string, a ...any) pgx.Row {
	return x.maStepTx.QueryRow(ctx, sql, a...)
}

func (x *maSnapTx) Query(ctx context.Context, sql string, a ...any) (pgx.Rows, error) {
	return x.maStepTx.Query(ctx, sql, a...)
}

func (x *maSnapTx) Exec(ctx context.Context, sql string, a ...any) (pgconn.CommandTag, error) {
	return x.maStepTx.Exec(ctx, sql, a...)
}

func (x *maSnapTx) Rollback(ctx context.Context) error { return x.maStepTx.Rollback(ctx) }

func (x *maSnapTx) Commit(ctx context.Context) error { return x.maStepTx.Commit(ctx) }

// maSnapIniziatore: registra le opzioni e dà la transazione finta.
type maSnapIniziatore struct {
	opzioni []pgx.TxOptions
	tx      *maSnapTx
	errore  error
}

func (i *maSnapIniziatore) BeginTx(_ context.Context, o pgx.TxOptions) (pgx.Tx, error) {
	i.opzioni = append(i.opzioni, o)
	if i.errore != nil {
		return nil, i.errore
	}
	return i.tx, nil
}

var maRO = pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}

func TestNelloSnapshotUnaTransazioneInSolaLettura(t *testing.T) {
	errF := errors.New("errore della funzione")
	for _, c := range []struct {
		nome string
		f    func(pgx.Tx) error
	}{{"riuscita", func(pgx.Tx) error { return nil }}, {"con un errore", func(pgx.Tx) error { return errF }}} {
		tx := &maSnapTx{}
		ini := &maSnapIniziatore{tx: tx}
		var vista pgx.Tx
		err := nelloSnapshot(context.Background(), ini, func(t pgx.Tx) error { vista = t; return c.f(t) })
		if (c.nome == "riuscita") != (err == nil) {
			t.Errorf("%s: errore %v", c.nome, err)
		}
		if len(ini.opzioni) != 1 || ini.opzioni[0] != maRO {
			t.Errorf("%s: opzioni %+v, attese una volta %+v", c.nome, ini.opzioni, maRO)
		}
		if vista != pgx.Tx(tx) || tx.rollback != 1 || tx.commit != 0 {
			t.Errorf("%s: la funzione non ha avuto la transazione (%v), %d rollback, %d commit", c.nome, vista == pgx.Tx(tx), tx.rollback, tx.commit)
		}
	}
	errPool := errors.New("pool chiuso")
	chiamata := false
	if err := nelloSnapshot(context.Background(), &maSnapIniziatore{errore: errPool}, func(pgx.Tx) error { chiamata = true; return nil }); !errors.Is(err, ErrLettura) || !errors.Is(err, errPool) || chiamata {
		t.Errorf("la transazione che non si apre: %v (funzione chiamata: %v)", err, chiamata)
	}
	if err := nelloSnapshot(context.Background(), nil, func(pgx.Tx) error { chiamata = true; return nil }); !errors.Is(err, ErrLettura) || chiamata {
		t.Errorf("senza Pool: %v (funzione chiamata: %v)", err, chiamata)
	}
}

// Il savepoint: con le opzioni dello snapshot, Tx.Begin della transazione esterna (un SAVEPOINT, non una transazione
// nuova); con altre opzioni un errore e nessun savepoint.
func TestIlSavepointDelCaricatore(t *testing.T) {
	tx := &maSnapTx{}
	sp := savepoint{tx}
	if _, err := sp.BeginTx(context.Background(), maRO); err != nil || tx.savepoint != 1 {
		t.Fatalf("con le opzioni dello snapshot: %v, %d savepoint", err, tx.savepoint)
	}
	for _, o := range []pgx.TxOptions{{}, {AccessMode: pgx.ReadOnly}, {IsoLevel: pgx.RepeatableRead},
		{IsoLevel: pgx.Serializable, AccessMode: pgx.ReadOnly}, {IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadWrite}} {
		if _, err := sp.BeginTx(context.Background(), o); err == nil {
			t.Errorf("opzioni %+v: nessun errore", o)
		}
	}
	if tx.savepoint != 1 {
		t.Errorf("%d savepoint: le opzioni diverse non ne aprono", tx.savepoint)
	}
}

// leggiSnapshot e Esiste cominciano con GetThread sulla transazione dello snapshot.
func TestLoSnapshotCominciaDallaRFQ(t *testing.T) {
	errRete := errors.New("connessione persa")
	for _, c := range []struct {
		nome   string
		prima  error
		atteso error
	}{{"la RFQ non c'è", pgx.ErrNoRows, ErrRFQInesistente}, {"GetThread non risponde", errRete, ErrLettura}} {
		for _, chi := range []string{"leggiSnapshot", "Esiste"} {
			tx := &maSnapTx{maStepTx: maStepTx{risposte: []error{c.prima}}}
			ini := &maSnapIniziatore{tx: tx}
			s := Servizio{Regole: maRegolePronte(0), Pool: ini}
			var err error
			if chi == "Esiste" {
				err = s.Esiste(context.Background(), maUUID(1))
			} else {
				_, err = s.leggiSnapshot(context.Background(), maUUID(1))
			}
			if !errors.Is(err, c.atteso) {
				t.Errorf("%s, %s: %v, atteso %v", c.nome, chi, err, c.atteso)
			}
			if errors.Is(err, ErrRFQInesistente) && errors.Is(err, ErrLettura) {
				t.Errorf("%s, %s: %v è insieme RFQ inesistente e lettura non riuscita", c.nome, chi, err)
			}
			if len(tx.sql) != 1 || !strings.Contains(tx.sql[0], "GetThread") || tx.savepoint != 0 || tx.rollback != 1 || tx.commit != 0 {
				t.Errorf("%s, %s: letture %q, %d savepoint, %d rollback, %d commit; attesi solo GetThread, nessun savepoint, un rollback",
					c.nome, chi, tx.sql, tx.savepoint, tx.rollback, tx.commit)
			}
			if len(ini.opzioni) != 1 || ini.opzioni[0] != maRO {
				t.Errorf("%s, %s: opzioni %+v", c.nome, chi, ini.opzioni)
			}
		}
	}
	// la RFQ c'è: Esiste risponde nil con una lettura sola
	tx := &maSnapTx{}
	if err := (Servizio{Pool: &maSnapIniziatore{tx: tx}}).Esiste(context.Background(), maUUID(1)); err != nil || len(tx.sql) != 1 {
		t.Errorf("la RFQ c'è: %v, letture %q", err, tx.sql)
	}
	if err := (Servizio{}).Esiste(context.Background(), maUUID(1)); !errors.Is(err, ErrLettura) {
		t.Errorf("Esiste senza Pool: %v", err)
	}
}
