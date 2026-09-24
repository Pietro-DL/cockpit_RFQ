//go:build integrazione

// L4 — i nomi sul NAS dopo A4 (B8.A4a): la revisione nel nome, il progressivo, il lucchetto della
// cartella, la riserva dei nomi da parte degli spostamenti e degli orfani, le cartelle che non si
// tolgono finche' qualcuno le nomina.
//
// Nomi delle prove di A4.12 dell'addendum: 2, 3, 4, 7, 18, 55, 56, 58, e la parte L4 della 74.

package documenti

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/storage/nas"
)

// bancoNomi e' una RFQ con un messaggio: i file hanno un contenuto vero in staging, e un documento si
// conferma come fa la conferma, con il nome scelto sotto il lucchetto della cartella.
type bancoNomi struct {
	t        *testing.T
	p        *pgxpool.Pool
	q        *db.Queries
	ctx      context.Context
	thread   uuid.UUID
	utente   uuid.UUID
	msg      uuid.UUID
	staging  string
	cartella string // cartella_relativa della RFQ
	n        int
}

func nuovoBancoNomi(t *testing.T) *bancoNomi {
	t.Helper()
	p, q, ctx := preparaDB(t)
	b := &bancoNomi{t: t, p: p, q: q, ctx: ctx, staging: t.TempDir(), cartella: `ACME\WIP\2026 09 24 nomi`}
	var cliente, conv uuid.UUID
	b.riga(`INSERT INTO cliente (cartella_nas, ragione_sociale) VALUES ('ACME', 'Acme') RETURNING cliente_id`, &cliente)
	b.riga(`INSERT INTO thread_offerta (cliente_id, canale, data_inizio, oggetto, cartella_relativa, priorita)
		VALUES ($1, 'outlook', now(), 'RFQ nomi', $2, 1) RETURNING thread_id`, &b.thread, cliente, b.cartella)
	b.riga(`INSERT INTO conversazione (canale, chiave_esterna, primo_messaggio_il) VALUES ('outlook', 'CONV-NOMI', now()) RETURNING conversazione_id`, &conv)
	b.riga(`INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, direzione, data_evento, oggetto, thread_id)
		VALUES ('outlook', 'MSG-NOMI', $1, 'entrata', now(), 'prova', $2) RETURNING messaggio_id`, &b.msg, conv, b.thread)
	b.riga(`INSERT INTO utente (sigla, nome, ufficio, ruolo) VALUES ('NM', 'Prova', 'tecnico', 'operatore') RETURNING utente_id`, &b.utente)
	return b
}

func (b *bancoNomi) riga(sql string, dst any, arg ...any) {
	b.t.Helper()
	if err := b.p.QueryRow(b.ctx, sql, arg...).Scan(dst); err != nil {
		b.t.Fatalf("%v\n%s", err, sql)
	}
}

func (b *bancoNomi) esegui(sql string, arg ...any) {
	b.t.Helper()
	if _, err := b.p.Exec(b.ctx, sql, arg...); err != nil {
		b.t.Fatalf("%v\n%s", err, sql)
	}
}

// file mette in staging un contenuto nuovo e ne fa un allegato del messaggio.
func (b *bancoNomi) file(nome, contenuto string) string {
	b.t.Helper()
	b.n++
	percorso := filepath.Join(b.staging, fmt.Sprintf("%d_%s", b.n, nome))
	if err := os.WriteFile(percorso, []byte(contenuto), 0o644); err != nil {
		b.t.Fatal(err)
	}
	sha, n, err := nas.Sha256File(percorso)
	if err != nil {
		b.t.Fatal(err)
	}
	b.esegui(`INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, bytes, sha256, path_staging, stato, ricevuto_il)
		VALUES ($1, $2, $3, $4, 'file', $5, $6, $7, 'analizzato', now())`, b.msg, b.n, nome, strings.TrimPrefix(filepath.Ext(nome), "."), n, sha, percorso)
	return sha
}

// conferma crea il documento di un file come la conferma: cartella del tipo e del codice, nome sul NAS
// scelto sotto il lucchetto, nome originale in nome_file.
func (b *bancoNomi) conferma(tipo db.TipoDocumento, codice, rev, nome, contenuto string, comp uuid.NullUUID) db.Documento {
	b.t.Helper()
	sha := b.file(nome, contenuto)
	tx, err := b.p.Begin(b.ctx)
	if err != nil {
		b.t.Fatal(err)
	}
	defer tx.Rollback(b.ctx)
	q := db.New(tx)
	l, err := q.GetCartellaDocumento(b.ctx, tipo)
	if err != nil {
		b.t.Fatal(err)
	}
	cartella, err := CartellaDocumento(LayoutDocumento{Sottocartella: l.Sottocartella, PerCodice: l.PerCodice}, true, codice)
	if err != nil {
		b.t.Fatal(err)
	}
	ext := strings.TrimPrefix(filepath.Ext(nome), ".")
	percorso, err := ScegliPercorso(b.ctx, q, b.thread, cartella, NomeSulNas(tipo, codice, rev, ext, nome))
	if err != nil {
		b.t.Fatal(err)
	}
	d, err := q.InsertDocumento(b.ctx, db.InsertDocumentoParams{ThreadID: b.thread, ComponenteID: comp, Tipo: tipo,
		Codice: pgtype.Text{String: codice, Valid: codice != ""}, Rev: pgtype.Text{String: rev, Valid: rev != ""}, NomeFile: nome,
		Estensione: strings.ToLower(ext), Sha256: sha, PathRelativo: percorso, ConfermatoDa: b.utente})
	if err != nil {
		b.t.Fatal(err)
	}
	if _, err := coda.AccodaCopia(b.ctx, q, d.DocumentoID); err != nil {
		b.t.Fatal(err)
	}
	if err := tx.Commit(b.ctx); err != nil {
		b.t.Fatal(err)
	}
	return d
}

// ------------------------------------------------------------------ nomi con la revisione

// Prove 3 e 4: <CODICE>_REV_<REV>, _REV_ND per la revisione ignota, _2 per il secondo file con lo
// stesso nome base, tutti nella stessa cartella del codice; l'indice unico rifiuta una collisione
// scritta direttamente.
func TestINomiConLaRevisioneNonCollidono(t *testing.T) {
	conCapacita(t, tutto)
	b := nuovoBancoNomi(t)
	attesi := []struct {
		tipo                 db.TipoDocumento
		rev, nome, contenuto string
		percorso             string
	}{
		{db.TipoDocumentoDisegno2d, "B", "52920517 rev B.pdf", "foglio 1", `ELENCO DISEGNI\52920517\52920517_REV_B.pdf`},
		{db.TipoDocumentoDisegno2d, "b", "52920517 foglio 2.PDF", "foglio 2", `ELENCO DISEGNI\52920517\52920517_REV_B_2.pdf`},
		{db.TipoDocumentoCad3d, "", "assieme.STEP", "lo STEP", `ELENCO DISEGNI\52920517\52920517_REV_ND.step`},
		{db.TipoDocumentoDisegno2d, "C", "52920517 rev C.pdf", "rev C", `ELENCO DISEGNI\52920517\52920517_REV_C.pdf`},
		{db.TipoDocumentoCapitolato, "", "Capitolato.pdf", "capitolato", `CAPITOLATI\Capitolato.pdf`},
		{db.TipoDocumentoCapitolato, "", "capitolato.PDF", "un altro capitolato", `CAPITOLATI\capitolato_2.pdf`},
	}
	for _, a := range attesi {
		d := b.conferma(a.tipo, "52920517", a.rev, a.nome, a.contenuto, uuid.NullUUID{})
		if d.PathRelativo != a.percorso {
			t.Errorf("%s: percorso %q, atteso %q", a.nome, d.PathRelativo, a.percorso)
		}
		if d.NomeFile != a.nome {
			t.Errorf("%s: nome_file %q: il nome originale si perde", a.nome, d.NomeFile)
		}
	}
	// prova 4: una revisione nuova non crea una cartella nuova
	var cartelle int
	b.riga(`SELECT count(DISTINCT lower(regexp_replace(path_relativo, '\\[^\\]*$', ''))) FROM documento WHERE tipo <> 'capitolato'`, &cartelle)
	if cartelle != 1 {
		t.Errorf("le revisioni stanno in %d cartelle, attesa una", cartelle)
	}
	_, err := b.p.Exec(b.ctx, `INSERT INTO documento (thread_id, tipo, codice, nome_file, estensione, sha256, path_relativo, confermato_da)
		VALUES ($1, 'disegno_2d', '52920517', 'x.pdf', 'pdf', repeat('f', 64), 'elenco disegni\52920517\52920517_rev_b.PDF', $2)`, b.thread, b.utente)
	if err == nil || !strings.Contains(err.Error(), "ux_documento_percorso") {
		t.Errorf("l'indice unico doveva rifiutare la collisione, ottenuto %v", err)
	}
}

// Prova 2: la revisione nuova e la vecchia sono due file, e la copia della nuova non tocca i byte della vecchia.
func TestUnaRevisioneNonToccaIByteDellaVecchia(t *testing.T) {
	conCapacita(t, tutto)
	b := nuovoBancoNomi(t)
	radice := t.TempDir()
	s := &nas.Scrittore{Radice: radice}
	vecchia := b.conferma(db.TipoDocumentoDisegno2d, "D7", "A", "d7.pdf", "revisione A", uuid.NullUUID{})
	if _, err := CopiaSulNas(b.ctx, b.q, s, nil, vecchia.DocumentoID); err != nil {
		t.Fatal(err)
	}
	fileVecchio := filepath.Join(radice, "ACME", "WIP", "2026 09 24 nomi", "ELENCO DISEGNI", "D7", "D7_REV_A.pdf")
	prima, err := os.ReadFile(fileVecchio)
	if err != nil {
		t.Fatal(err)
	}
	nuova := b.conferma(db.TipoDocumentoDisegno2d, "D7", "B", "d7.pdf", "revisione B", uuid.NullUUID{})
	if _, err := CopiaSulNas(b.ctx, b.q, s, nil, nuova.DocumentoID); err != nil {
		t.Fatal(err)
	}
	dopo, err := os.ReadFile(fileVecchio)
	if err != nil || string(dopo) != string(prima) || string(prima) != "revisione A" {
		t.Errorf("la vecchia revisione e' cambiata: %q → %q (%v)", prima, dopo, err)
	}
	if b, err := os.ReadFile(filepath.Join(filepath.Dir(fileVecchio), "D7_REV_B.pdf")); err != nil || string(b) != "revisione B" {
		t.Errorf("la nuova revisione: %q %v", b, err)
	}
}

// Prova 7: un componente con due padri ha un documento, una copia, un file; la cartella dipende dal
// codice, non dall'albero.
func TestUnComponenteConDuePadriHaUnaSolaCopia(t *testing.T) {
	conCapacita(t, tutto)
	b := nuovoBancoNomi(t)
	var p1, p2, f uuid.UUID
	b.riga(`INSERT INTO componente (thread_id, codice, tipo, confermato_da) VALUES ($1, 'P1', 'finito', $2) RETURNING componente_id`, &p1, b.thread, b.utente)
	b.riga(`INSERT INTO componente (thread_id, codice, tipo, confermato_da) VALUES ($1, 'P2', 'finito', $2) RETURNING componente_id`, &p2, b.thread, b.utente)
	b.riga(`INSERT INTO componente (thread_id, codice, confermato_da) VALUES ($1, 'S1', $2) RETURNING componente_id`, &f, b.thread, b.utente)
	b.esegui(`INSERT INTO componente_relazione (thread_id, padre_id, figlio_id, qta, origine, confermato_da)
		VALUES ($1, $2, $4, 2, 'manuale', $5), ($1, $3, $4, 3, 'manuale', $5)`, b.thread, p1, p2, f, b.utente)
	d := b.conferma(db.TipoDocumentoDisegno2d, "S1", "", "s1.pdf", "il pezzo condiviso", uuid.NullUUID{UUID: f, Valid: true})
	if d.PathRelativo != `ELENCO DISEGNI\S1\S1_REV_ND.pdf` {
		t.Errorf("percorso: %s", d.PathRelativo)
	}
	var documenti, nCopie, archi int
	b.riga(`SELECT count(*) FROM documento WHERE componente_id = $1`, &documenti, f)
	b.riga(`SELECT count(*) FROM job WHERE tipo = 'copia_nas'`, &nCopie)
	b.riga(`SELECT count(*) FROM componente_relazione WHERE figlio_id = $1`, &archi, f)
	if documenti != 1 || nCopie != 1 || archi != 2 {
		t.Errorf("documenti %d, copie %d, archi %d: attesi 1, 1, 2", documenti, nCopie, archi)
	}
}

// ------------------------------------------------------------------ la riserva dei nomi (A4.3, R2.1)

// Prova 55: una riga aperta di nas_orfano tiene occupato il suo nome, anche con maiuscole diverse; dopo
// la risoluzione il nome torna libero.
func TestUnOrfanoApertoTieneOccupatoIlSuoNome(t *testing.T) {
	conCapacita(t, tutto)
	b := nuovoBancoNomi(t)
	n, err := b.q.InsertNasOrfano(b.ctx, db.InsertNasOrfanoParams{ThreadID: b.thread, Percorso: `ELENCO DISEGNI\52920517\52920517_REV_B.pdf`, Motivo: db.MotivoOrfanoRimozioneFallita})
	if err != nil || n != 1 {
		t.Fatalf("orfano: %d %v", n, err)
	}
	var id int64
	b.riga(`SELECT nas_orfano_id FROM nas_orfano`, &id)
	for _, p := range []string{`ELENCO DISEGNI\52920517\52920517_REV_B.pdf`, `elenco disegni\52920517\52920517_rev_b.PDF`} {
		occupato, err := b.q.PercorsoOccupato(b.ctx, db.PercorsoOccupatoParams{ThreadID: b.thread, Percorso: p})
		if err != nil || !occupato.Bool {
			t.Errorf("%s: occupato %v %v", p, occupato.Bool, err)
		}
	}
	if d := b.conferma(db.TipoDocumentoDisegno2d, "52920517", "B", "b.pdf", "uno", uuid.NullUUID{}); d.PathRelativo != `ELENCO DISEGNI\52920517\52920517_REV_B_2.pdf` {
		t.Errorf("con l'orfano aperto il nome doveva saltare: %s", d.PathRelativo)
	}
	if n, err := b.q.RisolviNasOrfano(b.ctx, db.RisolviNasOrfanoParams{NasOrfanoID: id, RisoltoDa: uuid.NullUUID{UUID: b.utente, Valid: true}}); err != nil || n != 1 {
		t.Fatalf("risoluzione: %d %v", n, err)
	}
	if d := b.conferma(db.TipoDocumentoDisegno2d, "52920517", "B", "c.pdf", "due", uuid.NullUUID{}); d.PathRelativo != `ELENCO DISEGNI\52920517\52920517_REV_B.pdf` {
		t.Errorf("risolto l'orfano il nome doveva tornare libero: %s", d.PathRelativo)
	}
}

// Prova 56: una sola riga aperta per (thread, lower(percorso)); InsertNasOrfano non fallisce e dice che
// la riga c'era gia'; dopo la risoluzione se ne apre un'altra.
func TestUnSoloOrfanoApertoPerPercorso(t *testing.T) {
	b := nuovoBancoNomi(t)
	inserisci := func(p string) int64 {
		t.Helper()
		n, err := b.q.InsertNasOrfano(b.ctx, db.InsertNasOrfanoParams{ThreadID: b.thread, Percorso: p, Motivo: db.MotivoOrfanoNonNostro})
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	if inserisci(`A\x.pdf`) != 1 || inserisci(`a\X.PDF`) != 0 {
		t.Fatal("la seconda riga aperta doveva essere rifiutata senza errore")
	}
	b.esegui(`UPDATE nas_orfano SET risolto_il = now()`)
	if inserisci(`a\X.PDF`) != 1 {
		t.Error("dopo la risoluzione se ne doveva poter aprire un'altra")
	}
	var righe int
	b.riga(`SELECT count(*) FROM nas_orfano`, &righe)
	if righe != 2 {
		t.Errorf("righe: %d", righe)
	}
}

// Prova 58: una transazione accoda uno spostamento verso 52920517_REV_B.pdf, un'altra conferma un file
// con lo stesso nome base nella stessa cartella. Con il lucchetto la seconda aspetta e riceve _2; senza
// il lucchetto tutte e due vedono il nome libero e lo prendono.
func TestDueScelteDiNomeConcorrentiNonPrendonoLoStessoPercorso(t *testing.T) {
	conCapacita(t, tutto)
	cartella := `ELENCO DISEGNI\52920517`
	nome := "52920517_REV_B.pdf"
	for _, conLucchetto := range []bool{true, false} {
		t.Run(fmt.Sprintf("lucchetto %v", conLucchetto), func(t *testing.T) {
			b := nuovoBancoNomi(t)
			// il documento che si spostera': sta nella cartella di un altro codice
			vecchio := b.conferma(db.TipoDocumentoDisegno2d, "AAA111", "B", "a.pdf", "da spostare", uuid.NullUUID{})

			tx1, err := b.p.Begin(b.ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx1.Rollback(b.ctx)
			q1 := db.New(tx1)
			var a string
			if conLucchetto {
				a, err = AccodaSpostamento(b.ctx, q1, vecchio, cartella, nome)
			} else {
				a, err = primoLibero(b.ctx, q1, b.thread, cartella, nome)
				if err == nil {
					_, err = coda.Accoda(b.ctx, q1, db.TipoJobSpostaNas, PayloadSpostamento{ThreadID: b.thread, DocumentoID: vecchio.DocumentoID,
						Da: vecchio.PathRelativo, A: a, Sha256: vecchio.Sha256}, coda.ChiaveSpostamento(vecchio.DocumentoID), 1)
				}
			}
			if err != nil {
				t.Fatal(err)
			}

			scelto := make(chan string, 1)
			go func() {
				tx2, err := b.p.Begin(b.ctx)
				if err != nil {
					scelto <- "errore: " + err.Error()
					return
				}
				defer tx2.Rollback(b.ctx)
				q2 := db.New(tx2)
				var p string
				if conLucchetto {
					p, err = ScegliPercorso(b.ctx, q2, b.thread, cartella, nome)
				} else {
					p, err = primoLibero(b.ctx, q2, b.thread, cartella, nome)
				}
				if err != nil {
					scelto <- "errore: " + err.Error()
					return
				}
				scelto <- p
			}()
			var secondo string
			if conLucchetto {
				// la seconda e' in fila dietro la prima: sceglie solo dopo il COMMIT
				select {
				case p := <-scelto:
					t.Fatalf("con il lucchetto la seconda scelta non doveva arrivare prima del COMMIT: %s", p)
				case <-time.After(300 * time.Millisecond):
				}
				if err := tx1.Commit(b.ctx); err != nil {
					t.Fatal(err)
				}
				secondo = <-scelto
			} else {
				secondo = <-scelto
				if err := tx1.Commit(b.ctx); err != nil {
					t.Fatal(err)
				}
			}
			primo := cartella + `\` + nome
			if a != primo {
				t.Fatalf("lo spostamento ha scelto %s", a)
			}
			if conLucchetto && secondo != cartella+`\52920517_REV_B_2.pdf` {
				t.Errorf("con il lucchetto la seconda scelta doveva ricevere _2: %s", secondo)
			}
			if !conLucchetto && secondo != primo {
				t.Errorf("senza il lucchetto ci si aspettava la collisione che il lucchetto impedisce: %s", secondo)
			}
		})
	}
}

// Un documento non si sposta mentre si sta gia' spostando: la seconda richiesta si rifiuta (A4.3, passo 0).
func TestNonSiAccodaUnSecondoSpostamento(t *testing.T) {
	conCapacita(t, tutto)
	b := nuovoBancoNomi(t)
	d := b.conferma(db.TipoDocumentoDisegno2d, "AAA111", "B", "a.pdf", "da spostare", uuid.NullUUID{})
	if _, err := AccodaSpostamento(b.ctx, b.q, d, `ELENCO DISEGNI\BBB222`, "BBB222_REV_B.pdf"); err != nil {
		t.Fatal(err)
	}
	if _, err := AccodaSpostamento(b.ctx, b.q, d, `ELENCO DISEGNI\CCC333`, "CCC333_REV_B.pdf"); !errors.Is(err, ErrSpostamentoInCorso) {
		t.Errorf("secondo spostamento: %v", err)
	}
	// e la riserva vale per tutti e due i nomi
	for _, p := range []string{d.PathRelativo, `ELENCO DISEGNI\BBB222\BBB222_REV_B.pdf`} {
		occupato, err := b.q.PercorsoOccupato(b.ctx, db.PercorsoOccupatoParams{ThreadID: b.thread, Percorso: p})
		if err != nil || !occupato.Bool {
			t.Errorf("%s non riservato: %v", p, err)
		}
	}
	// in shadow lo spostamento non si accoda: scrive sul NAS
	d2 := b.conferma(db.TipoDocumentoCapitolato, "", "", "cap.pdf", "capitolato", uuid.NullUUID{})
	conCapacita(t, coda.Capacita{})
	if _, err := AccodaSpostamento(b.ctx, b.q, d2, `ALTRO`, "cap.pdf"); !errors.Is(err, coda.ErrCapacitaSpenta) {
		t.Errorf("in shadow: %v", err)
	}
}

// ------------------------------------------------------------------ cartelle (A4.3) e .parte (R2.12)

// Prova 18: una cartella non si toglie se un documento, uno spostamento pendente (da o a) o un orfano
// aperto la nominano, ne' se su disco ha qualcosa; una cartella libera e vuota si toglie.
func TestNonSiRimuoveUnaCartellaReferenziata(t *testing.T) {
	conCapacita(t, tutto)
	b := nuovoBancoNomi(t)
	radice := t.TempDir()
	s := &nas.Scrittore{Radice: radice}
	crea := func(rel string) string {
		p := filepath.Join(radice, "ACME", "WIP", "2026 09 24 nomi", filepath.FromSlash(strings.ReplaceAll(rel, `\`, "/")))
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	rimuovi := func(rel string) error {
		tx, err := b.p.Begin(b.ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(b.ctx)
		if err := RimuoviCartella(b.ctx, db.New(tx), s, b.thread, rel); err != nil {
			return err
		}
		return tx.Commit(b.ctx)
	}
	d := b.conferma(db.TipoDocumentoDisegno2d, "DOC1", "A", "d.pdf", "documento", uuid.NullUUID{})
	crea(`ELENCO DISEGNI\DOC1`)
	if err := rimuovi(`ELENCO DISEGNI\DOC1`); !errors.Is(err, ErrCartellaReferenziata) {
		t.Errorf("cartella di un documento: %v", err)
	}
	if _, err := AccodaSpostamento(b.ctx, b.q, d, `ELENCO DISEGNI\DEST`, "DEST_REV_A.pdf"); err != nil {
		t.Fatal(err)
	}
	crea(`ELENCO DISEGNI\DEST`)
	if err := rimuovi(`ELENCO DISEGNI\DEST`); !errors.Is(err, ErrCartellaReferenziata) {
		t.Errorf("cartella dell'«a» di uno spostamento pendente: %v", err)
	}
	b.esegui(`UPDATE documento SET path_relativo = 'ALTROVE\d.pdf' WHERE documento_id = $1`, d.DocumentoID)
	if err := rimuovi(`ELENCO DISEGNI\DOC1`); !errors.Is(err, ErrCartellaReferenziata) {
		t.Errorf("cartella del «da» di uno spostamento pendente: %v", err)
	}
	b.esegui(`UPDATE job SET stato = 'fatto', chiuso_il = now() WHERE tipo = 'sposta_nas'`)
	if _, err := b.q.InsertNasOrfano(b.ctx, db.InsertNasOrfanoParams{ThreadID: b.thread, Percorso: `ELENCO DISEGNI\DOC1\d.pdf`, Motivo: db.MotivoOrfanoNonNostro}); err != nil {
		t.Fatal(err)
	}
	if err := rimuovi(`ELENCO DISEGNI\DOC1`); !errors.Is(err, ErrCartellaReferenziata) {
		t.Errorf("cartella di un orfano aperto: %v", err)
	}
	b.esegui(`UPDATE nas_orfano SET risolto_il = now()`)

	// libera nel database, ma su disco c'e' un file: resta
	cartella := crea(`ELENCO DISEGNI\DOC1`)
	if err := os.WriteFile(filepath.Join(cartella, "appunti.txt"), []byte("di qualcuno"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := rimuovi(`ELENCO DISEGNI\DOC1`); !errors.Is(err, ErrCartellaNonVuota) {
		t.Errorf("cartella con un file: %v", err)
	}
	if err := os.Remove(filepath.Join(cartella, "appunti.txt")); err != nil {
		t.Fatal(err)
	}
	if err := rimuovi(`ELENCO DISEGNI\DOC1`); err != nil {
		t.Errorf("una cartella libera e vuota doveva togliersi: %v", err)
	}
	if _, err := os.Stat(cartella); !os.IsNotExist(err) {
		t.Errorf("la cartella c'e' ancora: %v", err)
	}
}

// Prova 74, la parte L4: i token dei tentativi vivi vengono dal database; un .parte vivo tiene la
// cartella, uno morto e vecchio si toglie.
func TestIParteDeiTentativiViviTengonoLaCartella(t *testing.T) {
	conCapacita(t, tutto)
	b := nuovoBancoNomi(t)
	radice := t.TempDir()
	s := &nas.Scrittore{Radice: radice}
	vivo, morto := uuid.New(), uuid.New()
	b.esegui(`INSERT INTO job (tipo, worker_tipo, payload, stato, lease_token, lease_fino_a, avviato_il, worker_id)
		VALUES ('copia_nas', 'server', '{}', 'in_corso', $1, now() + interval '5 minutes', now(), 'prova')`, vivo)
	cartella := filepath.Join(radice, "ACME", "WIP", "2026 09 24 nomi", "SVILUPPI")
	if err := os.MkdirAll(cartella, 0o755); err != nil {
		t.Fatal(err)
	}
	vecchio := time.Now().Add(-3 * time.Hour)
	for _, tk := range []uuid.UUID{vivo, morto} {
		p := filepath.Join(cartella, "x.dxf.parte."+tk.String())
		if err := os.WriteFile(p, []byte("a meta'"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, vecchio, vecchio); err != nil {
			t.Fatal(err)
		}
	}
	tx, err := b.p.Begin(b.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(b.ctx)
	err = RimuoviCartella(b.ctx, db.New(tx), s, b.thread, "SVILUPPI")
	if !errors.Is(err, ErrCartellaNonVuota) {
		t.Fatalf("con un .parte vivo la cartella doveva restare: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cartella, "x.dxf.parte."+morto.String())); !os.IsNotExist(err) {
		t.Errorf("il .parte morto e vecchio doveva sparire: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cartella, "x.dxf.parte."+vivo.String())); err != nil {
		t.Errorf("il .parte vivo doveva restare: %v", err)
	}
}
