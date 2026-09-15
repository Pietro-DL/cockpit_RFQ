//go:build integrazione

// L4 — voce 1.11: elaborazione singola sul download degli allegati.
//
// «Elaborazione singola» qui vuol dire una cosa concreta: lo stesso contenuto non si scarica due
// volte. Ogni download è un giro in COM su Outlook — la parte più lenta e più fragile di tutta la
// catena — e una copia in più sul disco di staging.
//
// La guardia sta dentro jobs.AccodaStage e non nei gestori HTTP, così vale per ogni punto del server
// che chieda un download: il triage, il pannello degli allegati, «Riscarica». Se stesse in uno dei tre,
// gli altri due continuerebbero a scaricare due volte e nessuno se ne accorgerebbe, perché il secondo
// download riesce benissimo: produce solo lavoro inutile.
//
// I3 e I18 — la stessa mail in due o quattro caselle — appartengono alla fase 2: senza
// messaggio_casella lo stesso messaggio in due caselle non è nemmeno rappresentabile.
package jobs

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/db"
)

// stagingFinto è lo staging come lo vede il server: un elenco di percorsi che esistono. Non tocca il
// disco, così il test prova la guardia e non il file system della macchina che lo esegue.
type stagingFinto map[string]bool

func (s stagingFinto) Presente(percorso string) bool { return s[percorso] }

// messaggioConAllegato crea messaggio, satellite Outlook e un allegato. `path` e `sha` valorizzati
// significano «già sceso una volta»; se `path` è vuoto l'allegato non è mai stato scaricato.
func messaggioConAllegato(t *testing.T, ctx context.Context, p *pgxpool.Pool, chiave, sha, path string) (db.Allegato, db.Messaggio, db.MessaggioOutlook) {
	t.Helper()
	var convID, msgID, allID uuid.UUID
	if err := p.QueryRow(ctx, `INSERT INTO conversazione (canale, chiave_esterna, primo_messaggio_il)
		VALUES ('outlook', $1, now()) RETURNING conversazione_id`, "CONV-"+chiave).Scan(&convID); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, direzione, data_evento, oggetto)
		VALUES ('outlook', $1, $2, 'entrata', now(), 'RFQ 1.11') RETURNING messaggio_id`,
		"<"+chiave+"@acme.example>", convID).Scan(&msgID); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, `INSERT INTO messaggio_outlook (messaggio_id, entry_id, store_id, cartella)
		VALUES ($1, $2, 'STORE-1', 'Posta in arrivo')`, msgID, "ENTRY-"+chiave); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, origine,
		bytes, sha256, path_staging, stato, ricevuto_il)
		VALUES ($1, 1, 'disegno.pdf', 'pdf', 'file', 'outlook', 2048,
		        NULLIF($2,''), NULLIF($3,''), CASE WHEN $3 = '' THEN 'grezzo' ELSE 'in_staging' END::stato_allegato, now())
		RETURNING allegato_id`, msgID, sha, path).Scan(&allID); err != nil {
		t.Fatal(err)
	}
	q := db.New(p)
	a, err := q.GetAllegato(ctx, allID)
	if err != nil {
		t.Fatal(err)
	}
	m, err := q.GetMessaggio(ctx, msgID)
	if err != nil {
		t.Fatal(err)
	}
	o, err := q.GetMessaggioOutlook(ctx, msgID)
	if err != nil {
		t.Fatal(err)
	}
	return a, m, o
}

func contaJobStage(t *testing.T, ctx context.Context, p *pgxpool.Pool, allegatoID uuid.UUID) int {
	t.Helper()
	var n int
	if err := p.QueryRow(ctx, `SELECT count(*) FROM job WHERE chiave_idempotenza = $1`,
		"stage:"+allegatoID.String()).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// Il file di questo allegato è al suo posto: non c'è niente da scaricare. Prima della voce 1.11 un
// secondo clic su «Scarica» riportava l'allegato a 'grezzo', cancellava path_staging dalla vista
// dell'operatore mettendo il marcatore «in coda», e rifaceva il giro in COM per riportare lo stesso
// identico file.
func TestNienteDownloadSeIlFileEGiaInStaging(t *testing.T) {
	p, q, ctx := preparaDB(t)
	percorso := `C:\staging\abc\disegno.pdf`
	a, m, o := messaggioConAllegato(t, ctx, p, "gia-presente", shaProva, percorso)
	st := stagingFinto{percorso: true}

	esito, j, err := AccodaStage(ctx, q, st, a, m, o, 1)
	if err != nil {
		t.Fatal(err)
	}
	if esito != StageGiaPresente {
		t.Errorf("esito = %q, atteso %q", esito, StageGiaPresente)
	}
	if j != nil {
		t.Error("è stato accodato un download per un file che c'è già")
	}
	if n := contaJobStage(t, ctx, p, a.AllegatoID); n != 0 {
		t.Errorf("job stage_allegato in coda = %d, attesi 0", n)
	}
	dopo, err := q.GetAllegato(ctx, a.AllegatoID)
	if err != nil {
		t.Fatal(err)
	}
	if dopo.Stato != db.StatoAllegatoInStaging || dopo.Errore.Valid {
		t.Errorf("lo stato dell'allegato è stato toccato inutilmente: stato=%s errore=%q", dopo.Stato, dopo.Errore.String)
	}
}

// Lo stesso disegno allegato a due richieste diverse è lo stesso file. Il secondo non si scarica: si
// riusa il percorso del primo. Questa è la metà della voce 1.11 che si può provare già alla fase 1,
// perché lo sha256 del secondo allegato lo conosciamo (il file era sceso una volta e poi è stato tolto
// dallo staging, per esempio dalla retention).
func TestStessoContenutoGiaInStagingNonSiScaricaDueVolte(t *testing.T) {
	p, q, ctx := preparaDB(t)
	percorso := `C:\staging\primo\disegno.pdf`
	_, _, _ = messaggioConAllegato(t, ctx, p, "primo", shaProva, percorso)
	secondo, m2, o2 := messaggioConAllegato(t, ctx, p, "secondo", shaProva, `C:\staging\secondo\disegno.pdf`)

	// solo il file del PRIMO esiste davvero
	st := stagingFinto{percorso: true}

	esito, j, err := AccodaStage(ctx, q, st, secondo, m2, o2, 1)
	if err != nil {
		t.Fatal(err)
	}
	if esito != StageRiusato {
		t.Fatalf("esito = %q, atteso %q", esito, StageRiusato)
	}
	if j != nil || contaJobStage(t, ctx, p, secondo.AllegatoID) != 0 {
		t.Error("il secondo allegato ha fatto partire un download: stesso contenuto, stesso file")
	}
	dopo, err := q.GetAllegato(ctx, secondo.AllegatoID)
	if err != nil {
		t.Fatal(err)
	}
	if dopo.PathStaging.String != percorso {
		t.Errorf("path_staging = %q, atteso %q", dopo.PathStaging.String, percorso)
	}
	if dopo.Stato != db.StatoAllegatoInStaging {
		t.Errorf("stato = %s, atteso in_staging", dopo.Stato)
	}
}

// Se nessuno dei due file c'è più, il download va fatto: la guardia deve fermare i doppioni, non i
// download che servono. È il caso di «Riscarica» dopo che lo staging è stato ripulito.
func TestFileSparitoDalloStagingSiRiscarica(t *testing.T) {
	p, q, ctx := preparaDB(t)
	_, _, _ = messaggioConAllegato(t, ctx, p, "gemello-sparito", shaProva, `C:\staging\x\disegno.pdf`)
	a, m, o := messaggioConAllegato(t, ctx, p, "da-riscaricare", shaProva, `C:\staging\y\disegno.pdf`)

	esito, j, err := AccodaStage(ctx, q, stagingFinto{}, a, m, o, 1) // nessun file esiste
	if err != nil {
		t.Fatal(err)
	}
	if esito != StageAccodato || j == nil {
		t.Fatalf("esito = %q, job = %v: il download doveva partire", esito, j)
	}
	dopo, err := q.GetAllegato(ctx, a.AllegatoID)
	if err != nil {
		t.Fatal(err)
	}
	if dopo.Errore.String != "in coda" {
		t.Errorf("marcatore di attesa = %q, atteso \"in coda\"", dopo.Errore.String)
	}
}

// Un allegato mai sceso, senza hash: si scarica. E due richieste di fila fanno un job solo, perché la
// chiave di idempotenza è per allegato: due operatori che premono «Scarica» insieme non devono
// mandare due volte lo stesso lavoro al worker.
func TestUnSoloDownloadPendentePerAllegato(t *testing.T) {
	p, q, ctx := preparaDB(t)
	a, m, o := messaggioConAllegato(t, ctx, p, "mai-sceso", "", "")

	esito, j, err := AccodaStage(ctx, q, stagingFinto{}, a, m, o, 1)
	if err != nil {
		t.Fatal(err)
	}
	if esito != StageAccodato || j == nil {
		t.Fatalf("primo download: esito = %q, job = %v", esito, j)
	}
	esito2, j2, err := AccodaStage(ctx, q, stagingFinto{}, a, m, o, 1)
	if err != nil {
		t.Fatal(err)
	}
	if esito2 != StageGiaInCoda || j2 != nil {
		t.Errorf("secondo download: esito = %q, job = %v (atteso %q, nessun job)", esito2, j2, StageGiaInCoda)
	}
	if n := contaJobStage(t, ctx, p, a.AllegatoID); n != 1 {
		t.Errorf("job stage_allegato per l'allegato = %d, atteso 1", n)
	}
}
