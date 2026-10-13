package distintabozza

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"promatec/cockpit/internal/platform/db"
)

// L1 — i casi che non vogliono il database: gli errori del database che non sono «non trovato» (RC-10), i controlli
// di Salva prima della transazione (mutante G07), la rilettura di uno snapshot con l'impronta sbagliata (G03), il
// confronto degli errori per codice e motivo (RC-03).

// dbGuasto: un DBTX che risponde a ogni lettura con lo stesso errore (connessione caduta, contesto annullato…).
type dbGuasto struct{ err error }

type rigaGuasta struct{ err error }

func (r rigaGuasta) Scan(...any) error { return r.err }

func (d dbGuasto) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, d.err
}
func (d dbGuasto) Query(context.Context, string, ...any) (pgx.Rows, error) { return nil, d.err }
func (d dbGuasto) QueryRow(context.Context, string, ...any) pgx.Row        { return rigaGuasta{d.err} }

// RC-10: solo «non trovato» e' un FUORI_RFQ; un guasto del database torna com'e' (in B2 un 5xx, non un 422 con una
// frase falsa per l'operatore).
func TestUnGuastoDelDatabaseNonEUnFuoriRFQ(t *testing.T) {
	ctx := context.Background()
	for _, guasto := range []error{errors.New("connessione caduta"), context.Canceled} {
		q := db.New(dbGuasto{guasto})
		_, err := prodottoDellaRfq(ctx, q, idDi("t"), idDi("p"), true)
		var e *Errore
		if errors.As(err, &e) || !errors.Is(err, guasto) {
			t.Errorf("prodottoDellaRfq con %v: %v", guasto, err)
		}
		if err := fonteDellaRfq(ctx, q, idDi("t"), fonteScena()); errors.As(err, &e) || !errors.Is(err, guasto) {
			t.Errorf("fonteDellaRfq con %v: %v", guasto, err)
		}
		if err := baseDellaRfq(ctx, q, idDi("t"), idDi("p"), Base{ConfermaID: pu(idDi("c")), Numero: 1, Firma: firmaProva}); errors.As(err, &e) || !errors.Is(err, guasto) {
			t.Errorf("baseDellaRfq con %v: %v", guasto, err)
		}
		if _, err := Leggi(ctx, q, idDi("t"), idDi("p")); errors.As(err, &e) || !errors.Is(err, guasto) {
			t.Errorf("Leggi con %v: %v", guasto, err)
		}
	}
	// e «non trovato» si', con il campo
	q := db.New(dbGuasto{pgx.ErrNoRows})
	if _, err := prodottoDellaRfq(ctx, q, idDi("t"), idDi("p"), true); !errors.Is(err, ErrFuoriRFQ) {
		t.Errorf("prodotto non trovato: %v", err)
	}
	if err := fonteDellaRfq(ctx, q, idDi("t"), fonteScena()); !errors.Is(err, ErrFuoriRFQ) {
		t.Errorf("allegato non trovato: %v", err)
	}
	if _, err := Leggi(ctx, q, idDi("t"), idDi("p")); !errors.Is(err, ErrRFQInesistente) {
		t.Errorf("RFQ non trovata in R1: %v", err)
	}
}

// nessunaTransazione: un Avviatore che fa fallire la prova se Salva apre una transazione.
type nessunaTransazione struct{ t *testing.T }

func (n nessunaTransazione) Begin(context.Context) (pgx.Tx, error) {
	n.t.Fatal("Salva ha aperto una transazione per un corpo che doveva rifiutare prima")
	return nil, nil
}

// G07 e gli altri controlli prima della transazione: lo snapshot di un altro prodotto, il vocabolario, la base
// mancante, la versione attesa di una bozza nuova.
func TestSalvaRifiutaPrimaDellaTransazione(t *testing.T) {
	sc := scena35(idDi("prodotto"), idDi)
	pool := nessunaTransazione{t}
	senzaBase := sc.snap.copia()
	senzaBase.VersioneBomBase, senzaBase.FirmaBase = nil, ""
	casi := []struct {
		nome  string
		s     Salvataggio
		campo string
	}{
		{"snapshot di un altro prodotto (G07)", Salvataggio{ProdottoID: idDi("altro"), Vocabolario: 1, Snapshot: grezzo(t, sc.snap)}, "snapshot.prodotto_id"},
		{"vocabolario 2", Salvataggio{ProdottoID: sc.prodotto, Vocabolario: 2, Snapshot: grezzo(t, sc.snap)}, "vocabolario"},
		{"vocabolario assente", Salvataggio{ProdottoID: sc.prodotto, Snapshot: grezzo(t, sc.snap)}, "vocabolario"},
		{"snapshot senza base", Salvataggio{ProdottoID: sc.prodotto, Vocabolario: 1, Snapshot: grezzo(t, senzaBase)}, "snapshot.versione_bom_base"},
		{"bozza nuova con versione attesa", Salvataggio{ProdottoID: sc.prodotto, Vocabolario: 1, VersioneAttesa: 3, Snapshot: grezzo(t, sc.snap)}, "versione_attesa"},
	}
	for _, c := range casi {
		_, err := Salva(context.Background(), pool, c.s)
		var e *Errore
		if !errors.As(err, &e) || e.Codice != CodiceCorpoNonValido || e.Campo != c.campo {
			t.Errorf("%s: %v", c.nome, err)
		}
	}
}

// G03, RC-01: uno snapshot salvato si rilegge solo se la sua forma canonica da' l'impronta registrata; altrimenti e'
// un guasto dei dati (ErrSnapshotIncoerente), non un *Errore del corpo.
func TestLoSnapshotSalvatoSiRicontrollaConLImpronta(t *testing.T) {
	sc := scena35(idDi("prodotto"), idDi)
	c, _ := sc.snap.Canonico()
	imp, _ := sc.snap.Impronta()
	if _, got, err := rileggiSnapshot(c, imp); err != nil || got != imp {
		t.Fatalf("rilettura con l'impronta giusta: %v", err)
	}
	// il testo che restituisce jsonb (spazi dopo «:» e «,», chiavi in un altro ordine) si rilegge alla stessa impronta
	var m map[string]any
	_ = json.Unmarshal(c, &m)
	conSpazi, _ := json.MarshalIndent(m, "", "  ")
	if _, got, err := rileggiSnapshot(conSpazi, imp); err != nil || got != imp {
		t.Errorf("rilettura del testo con gli spazi: %v", err)
	}
	_, _, err := rileggiSnapshot(c, shaProva('0'))
	var e *Errore
	if !errors.Is(err, ErrSnapshotIncoerente) || errors.As(err, &e) {
		t.Errorf("rilettura con l'impronta sbagliata: %v", err)
	}
	if _, _, err := rileggiSnapshot([]byte(`{"formato":2}`), imp); !errors.Is(err, ErrSnapshotIncoerente) {
		t.Errorf("rilettura di uno snapshot che non si legge: %v", err)
	}
}

// RC-03: i codici e i motivi del contratto rev. 3, e il confronto con errors.Is per codice e motivo.
func TestICodiciDegliErroriSonoQuelliDelContratto(t *testing.T) {
	altra := giaAperta(db.DistintaBozza{BozzaID: idDi("b"), Stato: StatoAperta, Versione: 4, AggiornataDa: idDi("u")})
	if altra.Codice != "BOZZA_VERSIONE" || altra.Motivo != "altra_bozza_aperta" || altra.Conflitto == nil || altra.Conflitto.BozzaID != idDi("b") || !altra.EConflitto() {
		t.Errorf("seconda bozza aperta: %+v", altra)
	}
	if !errors.Is(altra, ErrConflittoVersione) || !errors.Is(altra, ErrAltraBozzaAperta) {
		t.Error("errors.Is per codice e per codice + motivo")
	}
	vecchia := conflittoVersione(db.DistintaBozza{Stato: StatoAperta, Versione: 5}, 4)
	chiusa := conflittoVersione(db.DistintaBozza{Stato: StatoAbbandonata, Versione: 5}, 5)
	if vecchia.Motivo != "versione_diversa" || chiusa.Motivo != "bozza_chiusa" || errors.Is(vecchia, ErrAltraBozzaAperta) {
		t.Errorf("motivi: %q %q", vecchia.Motivo, chiusa.Motivo)
	}
	nr := nonRiconciliate("azioni", "x")
	if nr.Codice != "CORPO_NON_VALIDO" || nr.Motivo != "azioni_non_riconciliate" || !errors.Is(nr, ErrCorpoNonValido) || !errors.Is(nr, ErrAzioniNonRiconciliate) || nr.EConflitto() {
		t.Errorf("azioni non riconciliate: %+v", nr)
	}
	var nessuno *Errore
	if errors.Is(nessuno, ErrCorpoNonValido) || ErrCorpoNonValido.Is(nessuno) {
		t.Error("un *Errore nil non e' un CORPO_NON_VALIDO (e non manda in panico errors.Is)")
	}
	if errors.Is(corpo("x", "y"), ErrAzioniNonRiconciliate) {
		t.Error("un CORPO_NON_VALIDO senza motivo non e' azioni_non_riconciliate")
	}
	if (&Errore{Codice: CodiceBaseSuperata}).EConflitto() != true {
		t.Error("BASE_SUPERATA e' un conflitto (409)")
	}
	// il campo «rebase» della richiesta e l'azione rebase
	b := Base{Numero: 0, Contatore: 3, Firma: firmaProva}
	if e := rebaseCoerente(nil, nil); e != nil {
		t.Error(e)
	}
	if e := rebaseCoerente(&Rebase{Base: b, Motivo: "m"}, nil); e == nil || e.Campo != "rebase" {
		t.Errorf("campo rebase senza azione: %v", e)
	}
	if e := rebaseCoerente(nil, &RebaseDichiarato{Base: b, Motivo: "m"}); e == nil || e.Campo != "rebase" {
		t.Errorf("azione rebase senza campo: %v", e)
	}
	if e := rebaseCoerente(&Rebase{Base: b, Motivo: " m "}, &RebaseDichiarato{Base: b, Motivo: "m"}); e != nil {
		t.Errorf("rebase coerente: %v", e)
	}
	altraBase := b
	altraBase.Contatore = 4
	if e := rebaseCoerente(&Rebase{Base: altraBase, Motivo: "m"}, &RebaseDichiarato{Base: b, Motivo: "m"}); e == nil {
		t.Error("campo rebase con un'altra base")
	}
}
