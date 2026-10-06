//go:build integrazione

// L4 — il caricatore sul database di prova (A1c-L4S-02…07, la parte di B1; piano A, 6.4.2 e 6.7.3; contratto di A1c,
// §7, famiglia B1): una transazione sola, REPEATABLE READ READ ONLY, controllata con i due SHOW, tutto sulla stessa
// connessione, ROLLBACK per ultimo, nessun testo vietato nel registro SQL e nessuna differenza nel database; con
// Tutti l'elenco delle RFQ si legge dentro la transazione; un iniziatore che non dà la sola lettura ferma tutto
// prima delle letture di dominio; una scrittura nella transazione del caricatore è rifiutata (25006); i fatti sono
// quelli della terna corrente, e senza terna non ce ne sono; il motivo di completezza è quello della 0020; due
// fotografie della stessa scena hanno la stessa impronta, e una analisi nuova la cambia.
//
// La scena è inventata (ACME): nessun dato reale, il repository è pubblico.

package caricatore

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	risorse "promatec/cockpit"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/migrazioni"
	"promatec/cockpit/internal/platform/testutil"
)

func ultimaMigrazione(t *testing.T) int {
	t.Helper()
	migs, err := migrazioni.Elenca(risorse.FS)
	if err != nil {
		t.Fatal(err)
	}
	return migs[len(migs)-1].Versione
}

// controllaTransazione: il registro di un Carica riuscito comincia con il BEGIN in sola lettura e repeatable read e
// con i due SHOW, finisce con il ROLLBACK, sta tutto su una connessione e non ha testi vietati.
func controllaTransazione(t *testing.T, reg *testutil.RegistroSQL) []testutil.VoceSQL {
	t.Helper()
	voci := reg.Voci()
	if len(voci) < 5 {
		t.Fatalf("registro troppo corto: %v", voci)
	}
	begin := strings.ToLower(voci[0].SQL)
	if !strings.HasPrefix(begin, "begin") || !strings.Contains(begin, "repeatable read") || !strings.Contains(begin, "read only") {
		t.Errorf("il primo testo non è il BEGIN in sola lettura e repeatable read: %q", voci[0].SQL)
	}
	if voci[1].SQL != sqlSolaLettura || voci[2].SQL != sqlIsolamento {
		t.Errorf("dopo il BEGIN non ci sono i due SHOW: %q, %q", voci[1].SQL, voci[2].SQL)
	}
	if ultimo := strings.ToLower(strings.TrimSpace(voci[len(voci)-1].SQL)); ultimo != "rollback" {
		t.Errorf("l'ultimo testo non è il ROLLBACK: %q", voci[len(voci)-1].SQL)
	}
	for _, v := range voci {
		if v.Connessione != voci[0].Connessione {
			t.Errorf("un testo su un'altra connessione (%d invece di %d): %q", v.Connessione, voci[0].Connessione, v.SQL)
		}
	}
	if v := reg.Vietati(); len(v) > 0 {
		t.Errorf("testi vietati nel registro: %q", v)
	}
	return voci
}

func TestCaricaInUnaTransazioneSolaInSolaLettura(t *testing.T) {
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	s := costruisciScena(t, p)
	ctx := context.Background()
	pr, reg := testutil.PoolConRegistro(t, testutil.DSN(t))
	prima := testutil.FotoDelDatabase(t, p)

	reg.Azzera()
	f, err := Carica(ctx, pr, Richiesta{Thread: []uuid.UUID{s.thread}, Messaggi: []uuid.UUID{s.fuori}, Sorgente: "127.0.0.1/acme_prova_test come prove"})
	if err != nil {
		t.Fatal(err)
	}
	voci := controllaTransazione(t, reg)
	for _, v := range voci {
		if strings.Contains(v.SQL, "-- name: ListRfqPerLaRiapertura ") {
			t.Error("senza Tutti l'elenco delle RFQ non si legge")
		}
	}
	if !f.Coerente || f.Origine != fotorfq.OrigineDSN || f.SchemaDB != ultimaMigrazione(t) || f.SolaLettura != "on" ||
		f.Isolamento != "repeatable read" || f.VersioneSchema != fotorfq.VersioneSchema || f.PresaIl.IsZero() || f.Sorgente == "" {
		t.Errorf("testata della fotografia: %+v", f)
	}
	if len(f.Thread) != 1 || f.Thread[0].ID != s.thread || len(f.FuoriRFQ) != 1 || f.FuoriRFQ[0].Messaggio.ID != s.fuori {
		t.Fatalf("perimetro: %d thread, %d messaggi fuori RFQ", len(f.Thread), len(f.FuoriRFQ))
	}
	if d := testutil.Differenze(prima, testutil.FotoDelDatabase(t, p)); len(d) > 0 {
		t.Errorf("il caricatore ha cambiato il database: %v", d)
	}
	// la fotografia esce già nell'ordine canonico
	prima1 := inJSON(t, f)
	f.Ordina()
	if inJSON(t, f) != prima1 {
		t.Error("la fotografia di Carica non era nell'ordine di Ordina")
	}

	// con Tutti l'elenco delle RFQ si legge nella stessa transazione
	reg.Azzera()
	tutte, err := Carica(ctx, pr, Richiesta{Tutti: true})
	if err != nil {
		t.Fatal(err)
	}
	voci = controllaTransazione(t, reg)
	letto := false
	for _, v := range voci[1 : len(voci)-1] {
		letto = letto || strings.Contains(v.SQL, "-- name: ListRfqPerLaRiapertura ")
	}
	if !letto {
		t.Error("con Tutti l'elenco delle RFQ non risulta letto dentro la transazione")
	}
	if len(tutte.Thread) != 2 || len(tutte.FuoriRFQ) != 0 {
		t.Errorf("con Tutti: %d thread (attesi 2), %d messaggi fuori RFQ", len(tutte.Thread), len(tutte.FuoriRFQ))
	}

	// un thread chiesto che non c'è è un errore, non un'assenza silenziosa
	if _, err := Carica(ctx, pr, Richiesta{Thread: []uuid.UUID{uuid.MustParse("00000000-0000-4000-8000-0000000000ff")}}); err == nil {
		t.Error("una RFQ che non esiste è stata saltata in silenzio")
	}
}

// iniziatoreSbagliato apre la transazione con le opzioni sue, qualunque cosa chieda il caricatore.
type iniziatoreSbagliato struct {
	p *pgxpool.Pool
	o pgx.TxOptions
}

func (i iniziatoreSbagliato) BeginTx(ctx context.Context, _ pgx.TxOptions) (pgx.Tx, error) {
	return i.p.BeginTx(ctx, i.o)
}

func TestCaricaSiFermaSenzaLaSolaLettura(t *testing.T) {
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	s := costruisciScena(t, p)
	ctx := context.Background()
	pr, reg := testutil.PoolConRegistro(t, testutil.DSN(t))
	for nome, o := range map[string]pgx.TxOptions{
		"lettura e scrittura":    {IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadWrite},
		"read committed":         {IsoLevel: pgx.ReadCommitted, AccessMode: pgx.ReadOnly},
		"le opzioni di default":  {},
		"serializable read only": {IsoLevel: pgx.Serializable, AccessMode: pgx.ReadOnly},
	} {
		t.Run(nome, func(t *testing.T) {
			reg.Azzera()
			_, err := Carica(ctx, iniziatoreSbagliato{p: pr, o: o}, Richiesta{Thread: []uuid.UUID{s.thread}})
			if !errors.Is(err, ErrNonInSolaLettura) {
				t.Fatalf("errore %v, atteso ErrNonInSolaLettura", err)
			}
			for _, v := range reg.Voci() {
				testo := strings.ToLower(strings.TrimSpace(v.SQL))
				if !strings.HasPrefix(testo, "begin") && testo != strings.ToLower(sqlSolaLettura) &&
					testo != strings.ToLower(sqlIsolamento) && testo != "rollback" {
					t.Errorf("una lettura prima del controllo della transazione: %q", v.SQL)
				}
			}
		})
	}
}

// La scrittura deliberata nella transazione del caricatore (RO-DOMINIO, scrittura_attraverso_loader_read_only_rifiutata):
// il ruolo delle prove potrebbe scrivere, la transazione no.
func TestLaScritturaNellaTransazioneDelCaricatoreERifiutata(t *testing.T) {
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	ctx := context.Background()
	tx, err := apriInSolaLettura(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, `UPDATE documento_proposta SET stato = stato WHERE false`)
	var pe *pgconn.PgError
	if !errors.As(err, &pe) || pe.Code != "25006" {
		t.Errorf("una scrittura nella transazione del caricatore: %v, atteso read_only_sql_transaction (25006)", err)
	}
}

func TestLaTernaEsatta(t *testing.T) {
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	s := costruisciScena(t, p)
	ctx := context.Background()
	f, err := Carica(ctx, p, Richiesta{Thread: []uuid.UUID{s.thread}, Messaggi: []uuid.UUID{s.fuori}})
	if err != nil {
		t.Fatal(err)
	}
	corrente := fotorfq.Terna{Versione: versioneCorrente, HashConfigurazione: hashCorrente}
	if f.Analizzatore == nil || *f.Analizzatore != corrente {
		t.Fatalf("terna della fotografia: %+v", f.Analizzatore)
	}
	th := threadDi(t, f, s.thread)
	step, ok := th.Fatti[shaStep]
	if !ok || step.Terna != corrente || strings.Contains(string(step.Payload), "vecchia") {
		t.Errorf("i fatti dello STEP non sono quelli della terna corrente: %+v", step)
	}
	if _, ok := th.Fatti[shaSenzaFatt]; ok {
		t.Error("un contenuto senza analisi ha dei fatti")
	}
	if _, ok := th.Fatti[shaSoloDoc]; !ok {
		t.Error("i fatti di un documento senza allegato nel thread mancano (T-04)")
	}
	if len(th.Fatti) != 3+len(varianti) {
		t.Errorf("fatti del thread: %d, attesi %d", len(th.Fatti), 3+len(varianti))
	}
	if x, ok := f.FuoriRFQ[0].Fatti[shaPDF]; !ok || x.Terna != corrente {
		t.Errorf("fatti del messaggio senza RFQ: %+v", f.FuoriRFQ[0].Fatti)
	}

	// senza l'analizzatore corrente: nessun fatto, la sezione assente e la diagnosi
	if _, err := p.Exec(ctx, `DELETE FROM analizzatore_corrente`); err != nil {
		t.Fatal(err)
	}
	f, err = Carica(ctx, p, Richiesta{Thread: []uuid.UUID{s.thread}, Messaggi: []uuid.UUID{s.fuori}})
	if err != nil {
		t.Fatal(err)
	}
	if f.Analizzatore != nil || len(f.Thread[0].Fatti) != 0 || len(f.FuoriRFQ[0].Fatti) != 0 {
		t.Errorf("senza analizzatore: terna %+v, %d fatti", f.Analizzatore, len(f.Thread[0].Fatti))
	}
	if f.Sezioni[fotorfq.SezioneFatti].Stato != fotorfq.StatoSezioneAssente {
		t.Errorf("sezione dei fatti: %+v", f.Sezioni[fotorfq.SezioneFatti])
	}
	if len(f.Diagnostiche) != 1 || f.Diagnostiche[0].Codice != fotorfq.CodiceNessunAnalizzatore {
		t.Errorf("diagnostiche: %+v", f.Diagnostiche)
	}
}

// Il motivo di completezza viene dalla definizione unica della 0020, la stessa della query MotivoParziale; per i
// fatti senza struttura (un PDF) non c'è.
func TestLaCompletezzaDelleStrutture(t *testing.T) {
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	s := costruisciScena(t, p)
	ctx := context.Background()
	f, err := Carica(ctx, p, Richiesta{Thread: []uuid.UUID{s.thread}})
	if err != nil {
		t.Fatal(err)
	}
	th := threadDi(t, f, s.thread)
	q := db.New(p)
	completi := 0
	for _, v := range varianti {
		x, ok := th.Fatti[v.sha]
		if !ok || x.MotivoParziale == nil {
			t.Errorf("%s: fatti %v, motivo nil", v.nome, ok)
			continue
		}
		struttura := json.RawMessage(v.struttura)
		atteso, err := q.MotivoParziale(ctx, &struttura)
		if err != nil {
			t.Fatal(err)
		}
		if *x.MotivoParziale != atteso {
			t.Errorf("%s: motivo %q, la 0020 dice %q", v.nome, *x.MotivoParziale, atteso)
		}
		if atteso == "" {
			completi++
		}
	}
	if completi != 1 {
		t.Errorf("strutture complete fra le varianti: %d, attesa 1 (la prova non distingue i casi)", completi)
	}
	if x := th.Fatti[shaPDF]; x.MotivoParziale != nil {
		t.Errorf("un PDF ha un motivo di completezza: %q", *x.MotivoParziale)
	}
}

// Due fotografie della stessa scena: la stessa impronta (determinismo, contratto §7, B1). Poi un'analisi nuova alla
// stessa terna (A-C12, la parte DB di A1c-L4S-07): Digest e CalcolatoIl cambiano, e l'impronta con loro.
func TestDueFotografieDellaStessaScena(t *testing.T) {
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	s := costruisciScena(t, p)
	ctx := context.Background()
	r := Richiesta{Tutti: true, Messaggi: []uuid.UUID{s.fuori}}
	f1, err := Carica(ctx, p, r)
	if err != nil {
		t.Fatal(err)
	}
	f2, err := Carica(ctx, p, Richiesta{Tutti: true, Messaggi: []uuid.UUID{s.fuori}, Sorgente: "un'altra sorgente"})
	if err != nil {
		t.Fatal(err)
	}
	h1, err := fotorfq.ImprontaFotografia(f1)
	if err != nil {
		t.Fatal(err)
	}
	if h2, _ := fotorfq.ImprontaFotografia(f2); h2 != h1 {
		t.Fatalf("due fotografie della stessa scena: %s e %s", h1, h2)
	}
	if d := fotorfq.ValidaFotografia(f1); len(d) != 0 {
		t.Errorf("la fotografia della scena ha diagnostiche di contratto: %+v", d)
	}

	if _, err := db.New(p).UpsertAnalisiFatti(ctx, db.UpsertAnalisiFattiParams{Sha256: shaStep, VersioneAnalizzatore: versioneCorrente,
		HashConfigurazione: hashCorrente, Fatti: json.RawMessage(`{"struttura": ` + strutturaSTEP(3, `["#1"]`, false, false) + `}`)}); err != nil {
		t.Fatal(err)
	}
	f3, err := Carica(ctx, p, r)
	if err != nil {
		t.Fatal(err)
	}
	a, b := threadDi(t, f1, s.thread).Fatti[shaStep], threadDi(t, f3, s.thread).Fatti[shaStep]
	if a.Digest == b.Digest || a.CalcolatoIl.Equal(b.CalcolatoIl) {
		t.Errorf("dopo l'analisi nuova: Digest %s → %s, CalcolatoIl %v → %v", a.Digest, b.Digest, a.CalcolatoIl, b.CalcolatoIl)
	}
	if h3, _ := fotorfq.ImprontaFotografia(f3); h3 == h1 {
		t.Error("l'analisi nuova non cambia l'impronta della fotografia")
	}
	// la prima fotografia, conservata, non cambia: è un valore in memoria
	if h, _ := fotorfq.ImprontaFotografia(f1); h != h1 {
		t.Error("la fotografia conservata è cambiata")
	}
}
